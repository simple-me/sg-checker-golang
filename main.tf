provider "aws" {
  region = "us-east-1"
}

resource "null_resource" "lambda_build" {

  provisioner "local-exec" {
    #command = "cd aws-sdk-scan-security-groups && CGO_ENABLED=0 go build main.go"
    command = "cd aws-sdk-scan-security-groups && go build main.go"
  }

}


data "archive_file" "lambda_zip" {
  depends_on = [null_resource.lambda_build]
  type        = "zip"
  source_file = "aws-sdk-scan-security-groups/main"
  output_path = "main.zip"
}

resource "aws_lambda_function" "sync_sgs" {
  filename         = "main.zip"
  function_name    = "security-group-sync"
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  role             = aws_iam_role.iam_for_lambda.arn
  handler          = "main"
  runtime          = "go1.x"
  timeout = 10
}