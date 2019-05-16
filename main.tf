
provider "aws" {
  region = "us-west-2"
}

resource "aws_lambda_permission" "allow_cloudwatch" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = "${aws_lambda_function.s3quota.arn}"
  principal     = "events.amazonaws.com"
  source_arn    = "${aws_cloudwatch_event_rule.lambda.arn}"
}

resource "aws_cloudwatch_event_rule" "lambda" {
  name                = "s3quota"
  description         = "runs a lambda function every minute"
  schedule_expression = "rate(1 minute)"
}

resource "aws_cloudwatch_event_target" "lambda" {
  target_id = "s3quota"
  rule      = "${aws_cloudwatch_event_rule.lambda.name}"
  arn       = "${aws_lambda_function.s3quota.arn}"
}

resource "aws_lambda_function" "s3quota" {
  function_name    = "s3quota"
  filename         = "s3quota.zip"
  handler          = "s3quota"
  source_code_hash = "${base64sha256(file("s3quota.zip"))}"
  role             = "${aws_iam_role.s3quota.arn}"
  runtime          = "go1.x"
  memory_size      = 128
  timeout          = 2
}

resource "aws_iam_role_policy" "ses_log_policy" {
  role = "${aws_iam_role.s3quota.id}"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": [
        "ses:*",
        "logs:*",
        "cloudwatch:*",
        "s3:*"
      ],
      "Effect": "Allow",
      "Resource": "*"
    }
  ]
}
EOF
}

resource "aws_iam_role" "s3quota" {
  name               = "s3quota"
  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "lambda.amazonaws.com"
      },
      "Action": "sts:AssumeRole",
      "Sid": ""
    }
  ]
}
POLICY
}
