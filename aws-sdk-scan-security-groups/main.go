package main

import (
	"context"
	"describe_security_groups/utils"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/spf13/viper"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/slack-go/slack"
)

var TABLE_NAME = os.Getenv("table_name")

func sendSlackMessage(message string) {
	OAUTH_TOKEN := os.Getenv("OAUTH_TOKEN")
	CHANNEL_ID := os.Getenv("CHANNEL_ID")

	api := slack.New(OAUTH_TOKEN)
	attachment := slack.Attachment{
		//Pretext: "Pretext",
		Text: message,
	}

	channelId, timestamp, err := api.PostMessage(
		CHANNEL_ID,
		slack.MsgOptionText("The following security group has a 0.0.0.0/0 rule", false),
		slack.MsgOptionAttachments(attachment),
		slack.MsgOptionAsUser(true),
	)

	if err != nil {
		log.Fatalf("%s\n", err)
	}

	log.Printf("Message successfully sent to Channel %s at %s\n", channelId, timestamp)
}

func returnCreds(region string) aws.Config {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	return cfg
}

func putDynamoItem(tableName string, securityGroup string, attributeName string) {
	svc := dynamodb.NewFromConfig(returnCreds("us-east-1"))
	list_items := scanDynamoTableItems(TABLE_NAME)

	//Put Item if SG retrieved from API does not exist in the DynamoDB table
	if !utils.IsElementExist(list_items, securityGroup) || len(list_items) == 0 {
		fmt.Printf("Sg does not exist, adding to table: %s", securityGroup)
		result, err := svc.PutItem(context.TODO(), &dynamodb.PutItemInput{TableName: aws.String(tableName),
			Item: map[string]types.AttributeValue{
				attributeName: &types.AttributeValueMemberS{Value: securityGroup},
			},
		})
		fmt.Println("")
		fmt.Println(result.ResultMetadata)

		if err != nil {
			log.Fatalf("get work unit failed, %v", err)
		}
	}
}

func deleteUnnecessarySG(tableName string, attributeName string, list_sgs []string) {
	//Delete Item if SG already in the table does not exist in the list recently retrieved from the DynamoDB
	//API
	svc := dynamodb.NewFromConfig(returnCreds("us-east-1"))
	list_items := scanDynamoTableItems(TABLE_NAME)
	for sg := range list_items {
		if !utils.IsElementExist(list_sgs, list_items[sg]) {
			fmt.Printf("sg %s from dynamodb table does no exist in list retrieved from the ec2 API", list_items[sg])
			fmt.Println("")
			out, err := svc.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					attributeName: &types.AttributeValueMemberS{Value: list_items[sg]},
				},
			})
			if err != nil {
				panic(err)
			}

			fmt.Println(out.ResultMetadata)
		}
	}
}

func scanDynamoTableItems(tableName string) []string {
	list_items := []string{}
	svc := dynamodb.NewFromConfig(returnCreds("us-east-1"))
	result, err := svc.Scan(context.TODO(), &dynamodb.ScanInput{TableName: aws.String(tableName)})
	if err != nil {
		if strings.Contains(err.Error(), "ResourceNotFoundException") {
			log.Fatalf("Table not found")
		} else {
			fmt.Println(err.Error())
		}
	}

	if err != nil {
		log.Fatalf("unable to unmarshal records: %v", err)
	}
	for _, b := range result.Items {
		for _, c := range b {
			g, err := json.Marshal(c)
			if err != nil {
				fmt.Println("tdhtr")
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(g, &raw); err != nil {
				panic(err)
			}
			list_items = append(list_items, fmt.Sprint(raw["Value"]))
		}
	}

	return list_items

}

type LambdaEvent struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type LambdaResponse struct {
	Message string `json:"message"`
}

func LambdaHandler(event LambdaEvent) (LambdaResponse, error) {
	list_sgs := []string{}
	svc := ec2.NewFromConfig(returnCreds("us-east-1"))

	result, err := svc.DescribeSecurityGroups(context.TODO(), &ec2.DescribeSecurityGroupsInput{})

	if err != nil {
		fmt.Println(err.Error())
	}

	for _, i := range result.SecurityGroups {
		fmt.Println(*i.GroupName, *i.GroupId)
		for _, j := range i.IpPermissions {
			if j.FromPort == nil {
				fmt.Println("security group does not have from to port rule")
			}

			for _, k := range j.IpRanges {

				viper.SetConfigName("config") // name of config file (without extension)
				viper.AddConfigPath(".")      // path to look for the config file in

				err = viper.ReadInConfig()
				if err != nil {
					fmt.Println("Config not found...")
				} else {
					fmt.Println("config found....")
					cidrs := viper.GetStringSlice("CIDR")
					fmt.Println(cidrs)
					if utils.IsElementExist(cidrs, *k.CidrIp) {
						if !utils.IsElementExist(list_sgs, *i.GroupId) {
							list_sgs = append(list_sgs, *i.GroupId)
						}

					}
				}
			}
		}
	}
	for sg := range list_sgs {
		putDynamoItem(TABLE_NAME, list_sgs[sg], "SecurityGroupId")
	}

	deleteUnnecessarySG(TABLE_NAME, "SecurityGroupId", list_sgs)
	return LambdaResponse{
		Message: fmt.Sprintf("%s is %d years old.", event.Name, event.Age),
	}, nil

}

func main() {
	lambda.Start(LambdaHandler)

}
