package main

import (
	"errors"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap"
	"time"
)

// Logging as global
var logger, _ = zap.NewProduction()

// export ECONOMATIC_SCALEUP_HOUR=8
// export ECONOMATIC_SCALEUP_MINUTE=0
// export ECONOMATIC_SCALEDOWN_HOUR=2
// export ECONOMATIC_SCALEDOWN_MINUTE=55
type Configuration struct {
	ScaleupHour     int `required:"true" split_words:"true"`
	ScaleupMinute   int `required:"true" split_words:"true"`
	ScaledownHour   int `required:"true" split_words:"true"`
	ScaledownMinute int `required:"true" split_words:"true"`
}

func (c Configuration) ValidRunTime(hour int, minute int) error {
	now := time.Now()
	proposed := time.Date(now.Year(),
		now.Month(),
		now.Day(),
		hour,
		minute,
		0,
		0,
		now.Location(),
	)

	if now.After(proposed) {
		return nil
	} else {
		return errors.New("Executing before scheduled time")
	}
}

func work() error {
	defer logger.Sync()

	// Parse configuration
	var config Configuration
	err := envconfig.Process("economatic", &config)
	if err != nil {
		return err
	}

	// Read the stored metadata to determine if this is a scale up or down run.
	// If we read a valid nothing from Dynamo assume this is the first ever run
	// *and* that this is a scale up run:
	//
	//   - If the time check passes, the guess was correct, although no records
	//     will be stored, so no scaling action will be performed.
	//
	//   - If the time check fails we'll succeed on the next attempt
	//
	metadata, err := getMetaData()
	if err != nil {
		return err
	}

	if metadata.RunType == "" {
		metadata.RunType = "UP"
	}

	// Action time!
	switch metadata.RunType {
	case "UP":
		// Check that we're executing when we should be
		err := config.ValidRunTime(config.ScaleupHour, config.ScaleupMinute)
		if err != nil {
			return err
		}

		// Scale up each group for which a record exists, and remove the record
		groups, err := loadGroups()
		if err == nil {
			for _, group := range groups {
				err := group.Restore()
				if err != nil {
					logger.Warn("failed to restore group",
						zap.String("name", group.Name),
						zap.Int64("minimum", group.Minimum),
						zap.Int64("desired", group.Desired),
					)
				}
				err = group.Delete() // Do this regardless
				if err != nil {
					logger.Warn("unable to remove record",
						zap.String("name", group.Name),
						zap.Int64("minimum", group.Minimum),
						zap.Int64("desired", group.Desired),
					)
				}
			}
		} else {
			logger.Error("Could not load state from DynamoDB")
			return err
		}
	case "DOWN":
		// Check that we're executing when we should be
		err := config.ValidRunTime(config.ScaledownHour, config.ScaledownMinute)
		if err != nil {
			return err
		}

		// Scale down for each discovered auto-scaling group
		groups, err := describeAutoScalingGroups()
		if err == nil {
			for _, group := range groups {
				// Save the state of each group
				err := group.Save()
				// Scale the ASG down to zero
				if err == nil {
					err := group.Zero()
					if err != nil {
						// Continue despite a failure to scale
						logger.Warn("Could not scale down group:",
							zap.String("name", group.Name),
							zap.Int64("minimum", group.Minimum),
							zap.Int64("desired", group.Desired),
						)
					}
				}
			}
		}
	default:
		return errors.New("Invalid run type")
	}

	// Persist new metadata ready for the next run
	err = metadata.Save()
	if err != nil {
		return err
	}

	// Exit
	return nil
}

func main() {
	lambda.Start(work)
}
