variable "aws_region" {
  description = "AWS region for deployment"
  type        = string
  default     = "ca-west-1"
}

variable "project_name" {
  description = "Project name used for resource naming"
  type        = string
  default     = "recipe-to-reminders"
}

variable "anthropic_api_key" {
  description = "Anthropic API key for Claude Vision fallback (optional)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "lambda_memory" {
  description = "Lambda function memory in MB"
  type        = number
  default     = 2048
}

variable "lambda_timeout" {
  description = "Lambda function timeout in seconds"
  type        = number
  default     = 60
}

variable "lambda_reserved_concurrency" {
  description = "Maximum concurrent Lambda executions"
  type        = number
  default     = 10
}

variable "lambda_web_adapter_version" {
  description = "Version of the Lambda Web Adapter layer. Find latest at https://github.com/awslabs/aws-lambda-web-adapter"
  type        = number
  default     = 25
}
