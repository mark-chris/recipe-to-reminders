# Tesseract Lambda layer (built locally via make layer)
resource "aws_lambda_layer_version" "tesseract" {
  filename            = "${path.module}/../layers/tesseract/tesseract-layer.zip"
  layer_name          = "${var.project_name}-tesseract"
  source_code_hash    = filebase64sha256("${path.module}/../layers/tesseract/tesseract-layer.zip")
  compatible_runtimes = ["provided.al2023"]

  compatible_architectures = ["arm64"]
}

# Lambda function
resource "aws_lambda_function" "api" {
  filename         = "${path.module}/../bootstrap.zip"
  function_name    = var.project_name
  role             = aws_iam_role.lambda.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = filebase64sha256("${path.module}/../bootstrap.zip")
  memory_size      = var.lambda_memory
  timeout          = var.lambda_timeout

  reserved_concurrent_executions = var.lambda_reserved_concurrency

  layers = [
    "arn:aws:lambda:${var.aws_region}:753240598075:layer:LambdaAdapterLayerArm64:25",
    aws_lambda_layer_version.tesseract.arn
  ]

  environment {
    variables = {
      # Lambda Web Adapter
      PORT                        = "8080"
      AWS_LAMBDA_EXEC_WRAPPER     = "/opt/bootstrap"
      READINESS_CHECK_PATH        = "/extract"
      # S3 storage
      S3_BUCKET                   = aws_s3_bucket.recipes.id
      S3_RECIPES_KEY              = "recipes.json"
      # Tesseract OCR
      TESSERACT_PSM               = "6"
      TESSERACT_LANG              = "eng"
      TESSDATA_PREFIX             = "/opt/share/tessdata"
      # Photo extraction
      PHOTO_STRATEGY              = "tesseract-first"
      CONFIDENCE_THRESHOLD        = "0.65"
      # Claude Vision fallback (optional)
      ANTHROPIC_API_KEY           = var.anthropic_api_key
      CLAUDE_FALLBACK_MAX_PER_MIN = "10"
    }
  }
}

# Allow API Gateway to invoke Lambda
resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.api.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.api.execution_arn}/*/*"
}
