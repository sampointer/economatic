# economatic [![Circle CI](https://circleci.com/gh/sampointer/economatic.svg?style=shield)](https://circleci.com/gh/sampointer/economatic) <a href="https://godoc.org/github.com/sampointer/economatic"><img src="https://godoc.org/github.com/sampointer/economatic?status.svg" alt="GoDoc"></a> <img src="https://goreportcard.com/badge/github.com/sampointer/economatic"><img align="right" src="/assets/note.png" alt="euro note" />

## Description
Economatic is an AWS Lambda and associated infrastructure that will scale
down and restore auto-scaling groups within your AWS accounts on a schedule.

It is common to wish to do this for cost-saving purposes: you may have
development infrastructure that is not used outside of office hours, for
example.

It is also useful for periodically verifying DR infrastructure in distinct AWS
accounts that might be kept in "warm storage": deployed but quiesced.

### Alternatives before you begin
#### AWS Instance Scheduler
Amazon have their own solution to this problem in [AWS Instance Scheduler][1].
You should understand the differences between this and the official solution
before implementing either.

Broadly speaking, AWS' offering works on individual EC2 instances. It requires
the instance shutdown behaviour to be `Stop` rather than `Terminate`. It does
not play nicely with hosts in auto-scaling groups, which will be replaced
having failed health checks. It requires you to tag instances to indicate
you wish to have them controlled by the scheduler.

Economatic does the inverse of much of this. It works at the auto-scaling
group level and has no regards for individual hosts. In many common
infrastructure designs this will mean that hosts are terminated and re-provisioned
during each cycle. This in turn ensures that provisioning mechanisms are
exercised, "snowflake" hosts are removed, and costs are not incurred for idle
EBS volumes.  Finally economatic will **control all auto-scaling groups in a
given region by default, unless they are tagged for exclusion.** In
organisations with many tens or hundreds of AWS accounts this approach is
generally much more productive to realising cost savings.

Finally, [AWS Instance Scheduler][1] includes a complex scheduler and
associated state. In contrast economatic intentionally has much simpler
scheduling capabilities, less state, and coarser granularity by operating at the
regional level. In that sense it is more suited to organisations with many AWS
accounts and a more centralized SRE or operations team seeking to enact broad
policy.

#### Auto-Scaling Group Scheduled Scaling
[Scheduled Scaling][5] accomplishes a very similar goal to Economatic. The
difference between the two systems is largely philosophical, and to a great
extent political.

Economatic aims to make periodic scale-downs an *aspect of the AWS account*
rather than a property of the *things that are built in it*.

In large organisations it is typical for some central body to create and
provision AWS accounts. They may hook into consolidated billing, configure IAM
and MFA across parent or child accounts, allocate instance reservations,
deploy security applicances and the like. Economatic is designed to be part of
such an arrangement.

Economatic is suitable for organisations like this where it is unreasonable to
expect every tier of every project written by every team to consider
conditional scale-down for non-production accounts.

### What's in the box?
This repository contains:

* The Golang source code for the economatic lambda, and tools to build it
* A Cloudformation template to create and configure all of the required
supporting infrastrucutre.

### How does it work?
Economatic executions are best thought of as a latch circuit that flips between
scaling up and down. State is stored in DynamoDB.

When the Lambda is executed the `economatic_metadata` table is queried for the
type of run ("UP" or "DOWN") that should be performed. It then takes that
action, manipulating state in the `economatic` table as is required.

Once all of the activities have been completed the type of the next run is
flipped and stored, ready for the next invocation.

A scale-down run will enumerate all auto-scaling groups in the region, store
their minimum and desired instance counts and then set these values to zero. A
scale-up run will reset the minimum and desired values for all stored
auto-scaling groups.


External influences, such as the continuous integration of Cloudformation
templates into running stacks, may cause problems when interacting with
economatic.

## Getting Started
Before you begin you should understand that, unless tags are applied to your
auto-scaling group economatic **will destroy all instances controlled by all
auto-scaling groups within the region into which it is
deployed**. You have been warned. No warrantly is given or implied. Use of the
contents of this repository, including these instructions, **is entirely at your
own risk.**

### Building the Code
*You may wish to skip this step and use a supported [release][4], which
already includes a pre-built binary package.*

1. Have a [functioning Go development environment][2]
2. On OS X ensure you've [exported the appropriate variables][3] in order to
build Linux binaries
3. From within the root of this repository run:

```bash
$ dep ensure && go build && zip economatic.zip economatic
```

### Tagging your auto-scale groups for exclusion
Economatic will look for a tag `economatic` with a value of `false` when
considering whether to exclude a group from any scaling activities. How you
choose to do this is entirely dependent on how you manage your infrastructure.
You may have to update Cloudformation templates, Terraform manifests or take
some other action.

Most importantly you should **ensure that these changes survive any future
update by those mechanisms**, lest you find excluded groups being included in
scale-downs in the future.

### Create the Cloudformation stack from the template
* Create an S3 bucket in which to store the Lambda runtime and upload it
```bash
$ aws s3api create-bucket --acl private --bucket economatic-YOUR_ORG --create-bucket-configuration LocationConstraint=eu-west-2
$ aws s3 cp ./economatic.zip s3://economatic-YOUR_ORG/economatic.zip --acl private
```
* Configure the cloudformation parameter file with the S3 bucket ARN and other
required information. A example can be found in the `cloudformation/`
subdirectory:
```json
[{"ParameterKey":"ExecutableBucket","ParameterValue":"economatic-YOUR_ORG"},{"ParameterKey":"ScaleUpHour","ParameterValue":"8"},{"ParameterKey":"ScaleUpMinute","ParameterValue":"00"},{"ParameterKey":"ScaleDownHour","ParameterValue":"19"},{"ParameterKey":"ScaleDownMinute","ParameterValue":"55"}]
```
* Create the Cloudformation stack
```bash
aws cloudformation create-stack --stack-name economatic --template-body file://cloudformation/economatic.yaml --parameters $(cat cloudformation/parameters.json) --capabilities CAPABILITY_IAM --region eu-west-2
```
* Load in the seed values
```bash
$ aws dynamodb batch-write-item --request-items file://cloudformation/seeds.json --region eu-west-3
```

* Conditionally run the Lambda manually.
When first installed economatic has no state from which to derive what it should do next. It therefore assumes that the first run should be a scale-up run. Given the lack of state this effectively means that no action will be taken. If you've created the stack during the day and intend for the first action to be a scale down that same evening execute the Lambda manually from the console. No actions will be performed and the state will be set to scale down upon the next, automatic, invocation.

## Scheduling

### Terminating instances out of office hours
An organisation with GMT and PST offices sharing a number of development
accounts may wish to scale down development stacks from 0300 UTC (1900 PST)
and restore them at 0800 UTC (0000 PST) in order to save the common 5 hours
shared idle time each day.

### Warm storage DR infrastructure
If you have additional AWS accounts with deployed but quiesced DR infrastructure
you may wish to exercise it periodically. economatic runs scheduled to
provision instances for 2 hours in a daily basis could be used.

[1]: https://aws.amazon.com/answers/infrastructure-management/instance-scheduler/
[2]: https://golang.org/doc/install
[3]: https://github.com/aws/aws-lambda-go#for-developers-on-linux-and-macos
[4]: https://github.com/sampointer/economatic/releases
[5]: https://docs.aws.amazon.com/autoscaling/ec2/userguide/schedule_time.html
