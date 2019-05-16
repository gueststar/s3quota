# s3quota -- limit monthly downloads from an S3 hosted web site

This code automatically disables public access to an Amazon S3 hosted
website or public bucket if the outgoing traffic reaches a monthly
quota set by you. It is meant to run periodically as an AWS Lambda
function up to once each minute. On each run, it compares the quota to
the number of bytes downloaded since the beginning of the month. If
that number exceeds the quota, then the function revokes public access
to the bucket. Access is restored automatically at the beginning of
the following month. Any time the site status changes, the function
automatically sends a notification by email to an address you have
specified.

The purpose is to limit your charges by setting the quota based on the
amount you're willing to spend. For example, if your budget is $8 a
month, then the quota should be approximately 100 GB. There are
several alternatives, but I wrote this code because none of them
suited me.

* Something similar could be triggered by an AWS billing alarm, but
  billing alarms have a minimum six hour sampling period. Six hours of
  unlimited AWS charges would take a big bite.
* A cheap virtual server from other providers costs a flat rate per
  month and shuts down automatically if it reaches a quota, but it
  might not withstand a sudden surge in legitimate demand. I expect
  demand to be extremely variable. A cheap server costs me money when
  it's idle, and dies just when it's needed most. It's also nothing
  but a liability for security.
* Cloudflare does a content delivery service with a generous free tier,
  but is hostile to Tor users. The smart tech-savvy people in my target
  audience are too important to risk alienating.
* Free web hosting platforms sometimes have content restrictions or
  built in tracking.

## Prerequisites

You need the following before setting up.

* an AWS account 
* an S3 bucket already configured for web hosting and public access
* sender and receiver email addresses already validated with Amazon SES
* the Go language
* Terraform (unless you want to figure out how to do everything in the
  configuration file the hard way)

The bucket hosting the web site must have paid CloudWatch metrics
enabled, which are an opt-in feature. Running this code once every
minute will set you back about 44 cents a month in CloudWatch charges,
but will be well under the free usage threshold for Lambda charges. If
metrics are not already enabled for your bucket, you can enable them
by either the AWS management console, or by a command like this one
with the AWS command line tools. Note that the same Id has to appear
twice in the command, and can be anything not clashing with one
already in use on the same bucket.

```console
$ aws s3api put-bucket-metrics-configuration \
--bucket my_website_bucket_name \
--id any_id \
--metrics-configuration "Id=any_id"
```

It may also save money to set finite retention periods for your
CloudWatch logs using the AWS management console. (I don't know if
there's a way it can be done by Terraform or the cli.)

## Setup

Change the region in the Terraform configuration file ```main.tf``` to
wherever the bucket is hosted, and edit the constants declared in
```main.go``` to reflect your region, bucket name, preferred quota,
and validated email addresses. Then run the following commands.

```console
$ terraform init
$ go get .
$ GOOS=linux GOARCH=amd64 go build -o s3quota main.go
$ zip -o s3quota.zip s3quota
$ terraform apply
```

## Maintenance

To adjust the quota or any other constants in ```main.go```, edit the source
and the run these commands.
```console
$ GOOS=linux GOARCH=amd64 go build -o s3quota main.go
$ zip -o s3quota.zip s3quota
$ aws lambda update-function-code \
--region us-west-2 \
--function-name s3quota \
--zip-file fileb://s3quota.zip
```
with your preferred region substituted for
us-west-2. 

## Teardown

This command takes down the Lambda function and unschedules the
CloudWatch event, but leaves the web site the way it is.
```console
$ terraform destroy
```
If the web site is down and you want to put it back up immediately
without reading a manual first, use this command with your bucket name
and the appropriate region.
```console
$ aws s3api --region=us-west-2 \
put-public-access-block --bucket my_website_bucket_name \
--public-access-block-configuration \
BlockPublicAcls=false,\
IgnorePublicAcls=false,\
BlockPublicPolicy=false,\
RestrictPublicBuckets=false
```
Note that the site will be taken down again if the function is still
scheduled and the traffic since the beginning of the month is still
over the quota.

## Caveats

If there are bugs in this code or if you deploy it incorrectly, you
could be on the hook for unexpected charges. The code is GPL with no
guarantees. I recommend testing it on a temporary bucket with a very
low quota that you deliberately exceed to validate the setup.

If the code works perfectly, there is still a latency of
about five minutes from the time charges are incurred until the
traffic is reported by CloudWatch metrics (on top of the one minute
between function invocations). In the nightmare scenario of a
perfectly synchronized world class DDoS attack hammering your site
continuously up to the advertised CloudFront limit of 40Gbps, by my
calculations between 1 and 2 terabytes could slip through before this
function kicks in, costing you something on the order of $100. 

If it's any consolation, the attacker probably would have to spend more
than that to acquire a botnet of that scale, although maybe botnet
prices will soon come down to a dime a dozen with enough poorly
secured IoT devices coming on line.