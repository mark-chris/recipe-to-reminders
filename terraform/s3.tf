resource "aws_s3_bucket" "recipes" {
  bucket = "${var.project_name}-recipes-${data.aws_caller_identity.current.account_id}"
}

resource "aws_s3_bucket_versioning" "recipes" {
  bucket = aws_s3_bucket.recipes.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "recipes" {
  bucket = aws_s3_bucket.recipes.id

  rule {
    id     = "expire-noncurrent-versions"
    status = "Enabled"

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "recipes" {
  bucket = aws_s3_bucket.recipes.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "recipes" {
  bucket = aws_s3_bucket.recipes.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_policy" "recipes_https_only" {
  bucket     = aws_s3_bucket.recipes.id
  depends_on = [aws_s3_bucket_public_access_block.recipes]

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "DenyInsecureTransport"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource = [
          aws_s3_bucket.recipes.arn,
          "${aws_s3_bucket.recipes.arn}/*"
        ]
        Condition = {
          Bool = {
            "aws:SecureTransport" = "false"
          }
        }
      }
    ]
  })
}
