output "api_url" {
  description = "API Gateway invoke URL"
  value       = aws_api_gateway_stage.prod.invoke_url
}

output "api_key" {
  description = "API key value for x-api-key header"
  value       = aws_api_gateway_api_key.app.value
  sensitive   = true
}

output "s3_bucket" {
  description = "S3 bucket name for recipes storage"
  value       = aws_s3_bucket.recipes.id
}
