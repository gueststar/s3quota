/* 
    aws lambda function to take down an s3 hosted static website for the
    rest of the month when total downloads hit a monthly quota

    copyright (c) 2019 Dennis Furey

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"fmt"
	"time"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

const (
	website_bucket_name  = "www.example.com"                 // name of the bucket where the static web site is stored
	region               = "us-west-2"                       // region where the bucket and the lambda function are hosted
	monthly_byte_quota   = 100.0e9                           // 100.0e9 = 100GB costs around $8 a month in transfer fees
	recipient            = "me@myemailprovider.com"          // send an email here when the site goes on or off line
	sender               = "quotawatcher@example.com"        // send it from here
)





func confirmation(err error) (events.APIGatewayProxyResponse, error) {

	// Exit the handler with a minimally informative status message for the log.

	body := "normal termination"
	if err != nil {
		body = "abnormal termination"
	}
	res := events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body: body,
	}
	return res, err
}






func bytes_served_this_month (svc *cloudwatch.CloudWatch, bucket_name string) (float64, error) {

	// Return the total number of bytes downloaded from bucket_name
	// since the beginning of the month. If the region is wrong, this
	// function probably returns zero instead of detecting the error.

	metric := cloudwatch.Metric{
		MetricName: aws.String("BytesDownloaded"),
		Namespace: aws.String("AWS/S3"),
		Dimensions: []*cloudwatch.Dimension {
			{Name: aws.String("BucketName"), Value: aws.String(bucket_name)},
			{Name: aws.String("FilterId"), Value: aws.String("EntireBucket")}}}
	query := cloudwatch.MetricDataQuery{
		Id: aws.String("i"),                                          // required or else
		ReturnData: aws.Bool(true),
		MetricStat: &cloudwatch.MetricStat{
			Metric: &metric,
			Period: aws.Int64(2678460),                                // 31 days and 1 minute
			Stat: aws.String(cloudwatch.StatisticSum)}}
	start_of_the_month := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	end_of_the_minute := time.Now().Truncate(time.Minute).Add(time.Minute)
	input := cloudwatch.GetMetricDataInput{
		StartTime: &start_of_the_month,
		EndTime: &end_of_the_minute,
		MetricDataQueries: []*cloudwatch.MetricDataQuery{ &query },
		ScanBy: aws.String(cloudwatch.ScanByTimestampDescending)}
	data, err := svc.GetMetricData(&input)
	if err != nil {
		return 0.0, err
	}
	result := 0.0                                              // Metrics are retrievable only if the site has been visited
	if data.MetricDataResults != nil {                            // this month, which currently means something like this.
		values := data.MetricDataResults[0].Values
		if values != nil {
			result = *values[0]
		}
	}
	return result, nil
}




func site_is_online (svc *s3.S3, bucket_name string) (bool,error) {

	// Return true if the site in bucket_name is publically accessible,
	// false otherwise. A bucket set up for static website hosting has
	// all four of these fields false.

	output, err := svc.GetPublicAccessBlock(&s3.GetPublicAccessBlockInput{Bucket: aws.String(bucket_name)})
	if err != nil {
		return false, err
	}
	offline := *output.PublicAccessBlockConfiguration.BlockPublicAcls
	offline = offline || *output.PublicAccessBlockConfiguration.BlockPublicPolicy
	offline = offline || *output.PublicAccessBlockConfiguration.IgnorePublicAcls
	offline = offline || *output.PublicAccessBlockConfiguration.RestrictPublicBuckets
	return ! offline, nil
}





func failed_taking_site_offline (svc *s3.S3, bucket_name string) error {

	// Make the site in bucket_name publically inaccessible.

	config := s3.PublicAccessBlockConfiguration{
		BlockPublicAcls: aws.Bool(true),
		BlockPublicPolicy: aws.Bool(true),
		IgnorePublicAcls: aws.Bool(true),
		RestrictPublicBuckets: aws.Bool(true)}
	input := s3.PutPublicAccessBlockInput{Bucket: aws.String(bucket_name), PublicAccessBlockConfiguration: &config}
	_, err := svc.PutPublicAccessBlock(&input)
	return err
}




func failed_putting_site_online (svc *s3.S3, bucket_name string) error {

	// Make the site in bucket_name publically accessible.

	config := s3.PublicAccessBlockConfiguration{
		BlockPublicAcls: aws.Bool(false),
		BlockPublicPolicy: aws.Bool(false),
		IgnorePublicAcls: aws.Bool(false),
		RestrictPublicBuckets: aws.Bool(false)}
	input := s3.PutPublicAccessBlockInput{Bucket: aws.String(bucket_name), PublicAccessBlockConfiguration: &config}
	_, err := svc.PutPublicAccessBlock(&input)
	return err
}




func email_is_unsendable (c *ses.SES, bucket_name string, bytes float64, online bool) error {

	// Send an email notification of a change in status about the site
	// in bucket_name. If online is true then assume the site was off
	// line until now, and vice versa.

	subject := "Website http://" + bucket_name
	body := subject
	if online {
		subject = subject + " is online"
		body = body + " was put back on line as of \n\n   " + time.Now().Format(time.RFC822Z)
		body = body + "\n\nbecause its quota of\n\n   " + fmt.Sprintf("%0.3e",monthly_byte_quota)
		body = body + " bytes per month\n\nexceeds the current count of\n\n   " + fmt.Sprintf("%.3e",bytes)
		body = body + " bytes\n\nserved this month.\n"
	} else {
		subject = subject + " is offline"
		body = body + " was taken off line as of \n\n   " + time.Now().Format(time.RFC822Z)
		body = body + "\n\nhaving exceeded its quota of\n\n   " + fmt.Sprintf("%0.3e",monthly_byte_quota)
		body = body + " bytes per month\n\nby serving a total of\n\n   " + fmt.Sprintf("%.3e",bytes)
		body = body + " bytes\n\nthis month.\n"
	}
	input := ses.SendEmailInput{
		Destination: &ses.Destination{ToAddresses: []*string{aws.String(recipient)}},
		Message: &ses.Message{
			Body: &ses.Body{Text: &ses.Content{Data: &body}},
			Subject: &ses.Content{Charset: aws.String("UTF-8"),Data: &subject}},
		Source: aws.String(sender)}
	_, err := c.SendEmail(&input)
	return err
}





func handler(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

/* 
   To reduce cloudwatch charges, exit immediately if the website is
   off line and today is not the first day of the month. Otherwise,
   compare the total amount of data downloaded since the beginning of
   the month to the quota. If it is over the quota and the site is on
   line, take the site down. If it is under the quota and the site is
   off line, put the site up. If the site status changes, send a
   notification by email.
*/

	sess, err := session.NewSession(&aws.Config{Region:aws.String(region)})
	if err != nil {
		return confirmation (err)
	}
	svc := s3.New(sess)
	online, err := site_is_online (svc, website_bucket_name)
	if err != nil {
		return confirmation (err)
	}
	if (! online) && (time.Now().Day() != 1) {
		return confirmation (nil)                       // save money by not querying the metric until next month
	}
	bytes, err := bytes_served_this_month(cloudwatch.New(sess), website_bucket_name)
	if err != nil {
		return confirmation (err)
	}
	if (bytes >= monthly_byte_quota) && online {
		err = failed_taking_site_offline (svc, website_bucket_name)
		if err == nil {
			err = email_is_unsendable (ses.New(sess), website_bucket_name, bytes, false)
		}
	}
	if (bytes < monthly_byte_quota) && ! online {
		err = failed_putting_site_online (svc, website_bucket_name)
		if err == nil {
			err = email_is_unsendable (ses.New(sess), website_bucket_name, bytes, true)
		}
	}
	return confirmation (err)
}



func main() {
	lambda.Start(handler)
}
