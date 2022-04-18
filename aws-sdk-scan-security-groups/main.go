package main

import (
	"context"
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

	"github.com/slack-go/slack"

	"describe_security_groups/utils"

	"github.com/aws/aws-lambda-go/lambda"
)

var list_sgs = []string{}

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

func createTable() {
	svc := dynamodb.NewFromConfig(returnCreds("us-east-1"))
	out, err := svc.CreateTable(context.TODO(), &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		TableName:   aws.String("my-table"),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
}

func putDynamoItem(tableName string, securityGroup string, attributeName string) {
	svc := dynamodb.NewFromConfig(returnCreds("us-east-1"))
	list_items := scanDynamoTableItems("tf-notes-table")

	//Put Item if SG retrieved from API does not exist in the DynamoDB table
	if !utils.IsElementExist(list_items, securityGroup) || len(list_items) == 0 {
		fmt.Printf("Sg does not exist, adding to table: %s", securityGroup)
		result, err := svc.PutItem(context.TODO(), &dynamodb.PutItemInput{TableName: aws.String(tableName),
			Item: map[string]types.AttributeValue{
				//"noteId": &types.AttributeValueMemberS{Value: "aaaa"},
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

func deleteUnnecessarySG(tableName string, attributeName string) {
	//Delete Item if SG already in the table does not exist in the list recently retrieved from the DynamoDB
	//API
	svc := dynamodb.NewFromConfig(returnCreds("us-east-1"))
	list_items := scanDynamoTableItems("tf-notes-table")
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
			} else {
				//fmt.Println(*j.FromPort)
			}
			for _, k := range j.IpRanges {
				if *k.CidrIp == "0.0.0.0/0" {
					if !utils.IsElementExist(list_sgs, *i.GroupId) {
						list_sgs = append(list_sgs, *i.GroupId)
					}

				}
			}
		}
	}
	for sg := range list_sgs {
		putDynamoItem("tf-notes-table", list_sgs[sg], "SecurityGroupId")
	}

	deleteUnnecessarySG("tf-notes-table", "SecurityGroupId")
	return LambdaResponse{
		Message: fmt.Sprintf("%s is %d years old.", event.Name, event.Age),
	}, nil
}

func main() {
	lambda.Start(LambdaHandler)

}
