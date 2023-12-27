module "security_group_checker" {
  source                     = "git::https://github.com/non-existing-organization/terraform_module_lambda_dynamodb.git?ref=master"
  output_path                = "sg-checker.zip"
  function_name              = "security_group_checker_lambda"
  table_name                 = "sg-checker-table"
  attribute_name             = "SecurityGroupId"
  schedule_expression        = "rate(1 minute)"
  handler                    = "main"
  dynamodb_policy_name       = "sg-checker-dynamodb-policy"
  cloudwatch_event_rule_name = "trigger-sg-checker-lambda"
  lambda_role_name           = "iam_for_lambda"
  lambda_runtime                    = "go1.x"
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  cw_event_is_enabled = var.cw_event_is_enabled
}

resource "null_resource" "lambda_build" {

  provisioner "local-exec" {
    command = "cd aws-sdk-scan-security-groups && CGO_ENABLED=0 go build main.go"
  }

  triggers = {
    timestamp = timestamp()
  }

}

data "archive_file" "lambda_zip" {
  depends_on  = [null_resource.lambda_build]
  type        = "zip"
  source_dir = "aws-sdk-scan-security-groups/"
  output_path = "sg-checker.zip"
}