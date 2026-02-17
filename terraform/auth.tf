# API key
resource "aws_api_gateway_api_key" "app" {
  name    = "${var.project_name}-key"
  enabled = true
}

# Usage plan with rate limiting
resource "aws_api_gateway_usage_plan" "app" {
  name = "${var.project_name}-usage-plan"

  api_stages {
    api_id = aws_api_gateway_rest_api.api.id
    stage  = aws_api_gateway_stage.prod.stage_name
  }

  throttle_settings {
    burst_limit = 10
    rate_limit  = 5
  }
}

# Link API key to usage plan
resource "aws_api_gateway_usage_plan_key" "app" {
  key_id        = aws_api_gateway_api_key.app.id
  key_type      = "API_KEY"
  usage_plan_id = aws_api_gateway_usage_plan.app.id
}
