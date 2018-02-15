package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/guregu/dynamo"
	"go.uber.org/zap"
	"strings"
)

type AutoscalingGroupRecord struct {
	Name    string
	Minimum int64
	Desired int64
}

func (a AutoscalingGroupRecord) String() string {
	return fmt.Sprintf("%s, Minimum: %d, Desired: %d",
		a.Name, a.Minimum, a.Desired)
}

func (a AutoscalingGroupRecord) Save() error {
	logger.Info("Storing",
		zap.String("name", a.Name),
		zap.Int64("minimum", a.Minimum),
		zap.Int64("desired", a.Desired),
	)
	db := dynamo.New(session.New())
	table := db.Table("economatic")
	err := table.Put(a).Run()
	return err
}

func (a AutoscalingGroupRecord) Zero() error {
	logger.Info("Zeroing",
		zap.String("name", a.Name),
		zap.Int64("minimum", a.Minimum),
		zap.Int64("desired", a.Desired),
	)
	err := updateAutoScalingGroup(a.Name, 0, 0)
	return err
}

func (a AutoscalingGroupRecord) Restore() error {
	logger.Info("Restoring",
		zap.String("name", a.Name),
		zap.Int64("minimum", a.Minimum),
		zap.Int64("desired", a.Desired),
	)
	err := updateAutoScalingGroup(a.Name, a.Minimum, a.Desired)
	return err
}

func (a AutoscalingGroupRecord) Delete() error {
	logger.Info("Deleting record",
		zap.String("name", a.Name),
		zap.Int64("minimum", a.Minimum),
		zap.Int64("desired", a.Desired),
	)
	db := dynamo.New(session.New())
	table := db.Table("economatic")
	err := table.Delete("Name", a.Name).Run()
	return err
}

func loadGroups() ([]AutoscalingGroupRecord, error) {
	var groups []AutoscalingGroupRecord
	db := dynamo.New(session.New())
	table := db.Table("economatic")
	err := table.Scan().All(&groups)
	return groups, err
}

// Log the error and continue if we're unable to work on any given ASG: it may
// have disappeared underneath us if we're very unlucky, for example.
// Regardless, the best thing is to continue to work on other groups to
// minimise any damage.
func updateAutoScalingGroup(name string, min int64, desired int64) error {
	sess := session.Must(session.NewSession())
	asg := autoscaling.New(sess)
	input := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(name),
		MinSize:              aws.Int64(min),
		DesiredCapacity:      aws.Int64(desired),
	}

	_, err := asg.UpdateAutoScalingGroup(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case autoscaling.ErrCodeScalingActivityInProgressFault:
				fmt.Println(autoscaling.ErrCodeScalingActivityInProgressFault, aerr.Error())
			case autoscaling.ErrCodeResourceContentionFault:
				fmt.Println(autoscaling.ErrCodeResourceContentionFault, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return err
	}

	return nil
}

func describeAutoScalingGroups() ([]AutoscalingGroupRecord, error) {
	var groups []AutoscalingGroupRecord
	var excluded bool

	sess := session.Must(session.NewSession())
	asg := autoscaling.New(sess)
	// FIXME: this doesn't do pagination, which it must
	input := &autoscaling.DescribeAutoScalingGroupsInput{}
	result, err := asg.DescribeAutoScalingGroups(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case autoscaling.ErrCodeInvalidNextToken:
				fmt.Println(autoscaling.ErrCodeInvalidNextToken, aerr.Error())
				return groups, err
			case autoscaling.ErrCodeResourceContentionFault:
				fmt.Println(autoscaling.ErrCodeResourceContentionFault, aerr.Error())
				return groups, err
			default:
				fmt.Println(aerr.Error())
				return groups, err
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
			return groups, err
		}
	}

	for _, group := range result.AutoScalingGroups {
		// Only work on groups not tagged economatic: false
		excluded = false
		for _, tag := range group.Tags {
			if strings.ToLower(*tag.Key) == "economatic" {
				if strings.ToLower(*tag.Value) == "false" {
					excluded = true
					logger.Info("Excluding via tagging",
						zap.String("name", *group.AutoScalingGroupName),
					)
				}
			}
		}

		if excluded != true {
			groups = append(groups, AutoscalingGroupRecord{
				Name:    *group.AutoScalingGroupName,
				Minimum: *group.MinSize,
				Desired: *group.DesiredCapacity,
			})
		}
	}
	return groups, nil
}
