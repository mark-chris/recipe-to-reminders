# AWS Deployment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deploy the Recipe to Reminders backend to AWS Lambda behind API Gateway with Tesseract OCR, S3 storage, API key authentication, and rate limiting.

**Architecture:** Lambda Web Adapter (AWS-published layer) translates API Gateway REST API events into HTTP requests for the existing Go server. Tesseract 5.x runs as a custom Lambda layer compiled for Amazon Linux 2023 ARM64. Terraform manages all infrastructure. A Makefile orchestrates build and deploy.

**Tech Stack:** Terraform (AWS provider), Docker (Tesseract cross-compilation), Make, Go cross-compilation (GOOS=linux GOARCH=arm64)

---

### Task 1: .gitignore Updates

Add build artifacts and Terraform state files to .gitignore.

**Files:**
- Modify: `.gitignore`

**Step 1: Update .gitignore**

Add these lines to `.gitignore`:

```
# Build artifacts
bootstrap.zip

# Lambda layers
layers/tesseract/tesseract-layer.zip

# Terraform
.terraform/
*.tfstate
*.tfstate.backup
.terraform.lock.hcl
```

Note: `bootstrap`, `terraform.tfvars`, `terraform.tfstate`, `terraform.tfstate.backup`, and `.terraform/` are already in `.gitignore`. This adds the zip artifacts and `.terraform.lock.hcl`.

**Step 2: Verify no tracked files match new patterns**

Run: `git status`
Expected: Only `.gitignore` modified.

**Step 3: Commit**

```bash
git add .gitignore
git commit -m "chore: add build artifacts and Terraform files to .gitignore"
```

---

### Task 2: Makefile

Create a Makefile with build, layer, deploy, destroy, and clean targets.

**Files:**
- Create: `Makefile`

**Step 1: Create the Makefile**

```makefile
.PHONY: build layer deploy destroy clean

BINARY := bootstrap
ZIP := bootstrap.zip
LAYER_DIR := layers/tesseract
LAYER_ZIP := $(LAYER_DIR)/tesseract-layer.zip
TF_DIR := terraform

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/lambda/
	zip $(ZIP) $(BINARY)

layer:
	cd $(LAYER_DIR) && bash build.sh

deploy: build
	cd $(TF_DIR) && terraform init && terraform apply

destroy:
	cd $(TF_DIR) && terraform destroy

clean:
	rm -f $(BINARY) $(ZIP) $(LAYER_ZIP)

fmt:
	cd $(TF_DIR) && terraform fmt

validate:
	cd $(TF_DIR) && terraform validate
```

**Step 2: Test the build target**

Run: `make build`
Expected: Creates `bootstrap` (ARM64 ELF binary) and `bootstrap.zip`.

Run: `file bootstrap`
Expected: `bootstrap: ELF 64-bit LSB executable, ARM aarch64, ...`

**Step 3: Test the clean target**

Run: `make clean && ls bootstrap bootstrap.zip 2>&1`
Expected: `ls: cannot access 'bootstrap': No such file or directory` (files cleaned up)

**Step 4: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile for build, layer, and deploy targets"
```

---

### Task 3: Tesseract Lambda Layer Build Script

Create a Docker-based build script that compiles Tesseract 5.x with Leptonica for Amazon Linux 2023 ARM64 and packages it as a Lambda layer zip.

**Files:**
- Create: `layers/tesseract/build.sh`

**Step 1: Create directory structure**

```bash
mkdir -p layers/tesseract
```

**Step 2: Create the build script**

Create `layers/tesseract/build.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Build Tesseract 5.x Lambda layer for Amazon Linux 2023 ARM64.
# Requires Docker with QEMU binfmt support for ARM64 emulation.
#
# Output: tesseract-layer.zip (~15-20MB) in this directory.
#
# Usage: cd layers/tesseract && bash build.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_ZIP="${SCRIPT_DIR}/tesseract-layer.zip"
CONTAINER_NAME="tesseract-layer-builder"
LEPTONICA_VERSION="1.84.1"
TESSERACT_VERSION="5.5.0"
TESSDATA_URL="https://github.com/tesseract-ocr/tessdata_best/raw/main/eng.traineddata"

echo "==> Building Tesseract ${TESSERACT_VERSION} Lambda layer for ARM64..."
echo "    Leptonica: ${LEPTONICA_VERSION}"
echo "    This may take 10-15 minutes on x86 hosts (QEMU emulation)."

# Clean up any previous builder container
docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true

docker run --name "${CONTAINER_NAME}" --platform linux/arm64 \
  amazonlinux:2023 bash -c "
set -euo pipefail

# Install build dependencies
dnf groupinstall -y 'Development Tools'
dnf install -y cmake libtiff-devel libjpeg-turbo-devel libpng-devel \
  zlib-devel wget tar gzip

# Build Leptonica
cd /tmp
wget -q https://github.com/DanBloomberg/leptonica/releases/download/${LEPTONICA_VERSION}/leptonica-${LEPTONICA_VERSION}.tar.gz
tar xzf leptonica-${LEPTONICA_VERSION}.tar.gz
cd leptonica-${LEPTONICA_VERSION}
mkdir build && cd build
cmake .. -DCMAKE_INSTALL_PREFIX=/opt -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON
make -j\$(nproc)
make install

# Build Tesseract
cd /tmp
wget -q https://github.com/tesseract-ocr/tesseract/archive/refs/tags/${TESSERACT_VERSION}.tar.gz
tar xzf ${TESSERACT_VERSION}.tar.gz
cd tesseract-${TESSERACT_VERSION}
mkdir build && cd build
cmake .. -DCMAKE_INSTALL_PREFIX=/opt -DBUILD_SHARED_LIBS=OFF \
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
  -DLeptonica_DIR=/opt/lib64/cmake/leptonica \
  -DBUILD_TRAINING_TOOLS=OFF -DDISABLED_LEGACY_ENGINE=ON
make -j\$(nproc)
make install

# Download trained data
mkdir -p /opt/share/tessdata
wget -q -O /opt/share/tessdata/eng.traineddata ${TESSDATA_URL}

# Verify
/opt/bin/tesseract --version
echo '==> Tesseract built successfully'
"

# Copy artifacts out
echo "==> Extracting layer artifacts..."
STAGING=$(mktemp -d)
docker cp "${CONTAINER_NAME}:/opt/bin/tesseract" "${STAGING}/tesseract"
mkdir -p "${STAGING}/share/tessdata"
docker cp "${CONTAINER_NAME}:/opt/share/tessdata/eng.traineddata" "${STAGING}/share/tessdata/eng.traineddata"

# Also copy required shared libraries
mkdir -p "${STAGING}/lib"
docker cp "${CONTAINER_NAME}:/opt/lib64/." "${STAGING}/lib/" 2>/dev/null || true
docker cp "${CONTAINER_NAME}:/opt/lib/." "${STAGING}/lib/" 2>/dev/null || true

# Create layer zip with Lambda layer structure
# Lambda layers extract to /opt, so paths should be relative:
#   bin/tesseract -> /opt/bin/tesseract
#   share/tessdata/eng.traineddata -> /opt/share/tessdata/eng.traineddata
#   lib/ -> /opt/lib/ (shared libs if any)
echo "==> Creating layer zip..."
rm -f "${OUTPUT_ZIP}"
cd "${STAGING}"
mkdir -p bin
mv tesseract bin/
zip -r "${OUTPUT_ZIP}" bin/ share/ lib/

# Cleanup
docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
rm -rf "${STAGING}"

SIZE=$(du -h "${OUTPUT_ZIP}" | cut -f1)
echo "==> Layer zip created: ${OUTPUT_ZIP} (${SIZE})"
```

**Step 3: Verify script syntax**

Run: `bash -n layers/tesseract/build.sh`
Expected: No output (syntax OK).

**Step 4: Commit (do NOT run build.sh yet — it takes 10-15 min)**

```bash
git add layers/tesseract/build.sh
git commit -m "feat: add Tesseract Lambda layer build script for ARM64"
```

---

### Task 4: Terraform — Provider and Variables

Set up the Terraform foundation: provider configuration, variables, and a tfvars template.

**Files:**
- Create: `terraform/main.tf`
- Create: `terraform/variables.tf`
- Create: `terraform/terraform.tfvars.example`

**Step 1: Create terraform directory**

```bash
mkdir -p terraform
```

**Step 2: Create `terraform/main.tf`**

```hcl
terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = var.project_name
      ManagedBy = "terraform"
    }
  }
}
```

**Step 3: Create `terraform/variables.tf`**

```hcl
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
```

**Step 4: Create `terraform/terraform.tfvars.example`**

```hcl
# Copy this file to terraform.tfvars and fill in your values.
# terraform.tfvars is .gitignored — never commit it.

aws_region        = "ca-west-1"
anthropic_api_key = ""  # Optional: set to enable Claude Vision fallback
```

**Step 5: Validate Terraform syntax**

Run: `cd terraform && terraform init -backend=false && terraform validate`
Expected: `Success! The configuration is valid.`

Note: `terraform init -backend=false` skips provider download (faster, just validates syntax). Full `terraform init` happens at deploy time.

**Step 6: Commit**

```bash
git add terraform/main.tf terraform/variables.tf terraform/terraform.tfvars.example
git commit -m "feat: add Terraform provider config and variables"
```

---

### Task 5: Terraform — S3 Bucket

Create the S3 bucket with versioning, encryption, public access block, and HTTPS-only policy.

**Files:**
- Create: `terraform/s3.tf`

**Step 1: Create `terraform/s3.tf`**

```hcl
resource "aws_s3_bucket" "recipes" {
  bucket = "${var.project_name}-recipes"
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
  bucket = aws_s3_bucket.recipes.id

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
```

**Step 2: Validate**

Run: `cd terraform && terraform validate`
Expected: `Success! The configuration is valid.`

**Step 3: Commit**

```bash
git add terraform/s3.tf
git commit -m "feat: add S3 bucket with versioning, encryption, and access controls"
```

---

### Task 6: Terraform — IAM Role and Policy

Create the Lambda execution role with least-privilege S3 and CloudWatch Logs permissions.

**Files:**
- Create: `terraform/iam.tf`

**Step 1: Create `terraform/iam.tf`**

```hcl
# Lambda execution role
resource "aws_iam_role" "lambda" {
  name = "${var.project_name}-lambda"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

# S3 access — scoped to recipes.json only
resource "aws_iam_role_policy" "lambda_s3" {
  name = "s3-recipes-access"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ReadWriteRecipesObject"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:GetObjectVersion"
        ]
        Resource = "${aws_s3_bucket.recipes.arn}/recipes.json"
      },
      {
        Sid    = "ListVersionsForRecovery"
        Effect = "Allow"
        Action = [
          "s3:ListBucketVersions"
        ]
        Resource = aws_s3_bucket.recipes.arn
      }
    ]
  })
}

# CloudWatch Logs — scoped to this function's log group
resource "aws_iam_role_policy" "lambda_logs" {
  name = "cloudwatch-logs"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:*:log-group:/aws/lambda/${var.project_name}:*"
      }
    ]
  })
}
```

**Step 2: Validate**

Run: `cd terraform && terraform validate`
Expected: `Success! The configuration is valid.`

**Step 3: Commit**

```bash
git add terraform/iam.tf
git commit -m "feat: add Lambda IAM role with least-privilege S3 and logs permissions"
```

---

### Task 7: Terraform — Lambda Function and Layers

Create the Lambda function with the Web Adapter layer, Tesseract layer, and environment variables.

**Files:**
- Create: `terraform/lambda.tf`

**Step 1: Create `terraform/lambda.tf`**

```hcl
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
      PORT                      = "8080"
      AWS_LAMBDA_EXEC_WRAPPER   = "/opt/bootstrap"
      READINESS_CHECK_PATH      = "/extract"
      # S3 storage
      S3_BUCKET                 = aws_s3_bucket.recipes.id
      S3_RECIPES_KEY            = "recipes.json"
      # Tesseract OCR
      TESSERACT_PSM             = "6"
      TESSERACT_LANG            = "eng"
      TESSDATA_PREFIX           = "/opt/share/tessdata"
      # Photo extraction
      PHOTO_STRATEGY            = "tesseract-first"
      CONFIDENCE_THRESHOLD      = "0.65"
      # Claude Vision fallback (optional)
      ANTHROPIC_API_KEY         = var.anthropic_api_key
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
```

**Step 2: Validate**

Run: `cd terraform && terraform validate`
Expected: Validation may warn about missing zip files (expected — they don't exist until build). The HCL syntax should be valid.

**Step 3: Commit**

```bash
git add terraform/lambda.tf
git commit -m "feat: add Lambda function with Web Adapter and Tesseract layers"
```

---

### Task 8: Terraform — API Gateway REST API

Create the REST API with a catch-all proxy resource, Lambda integration, deployment, and stage.

**Files:**
- Create: `terraform/api-gateway.tf`

**Step 1: Create `terraform/api-gateway.tf`**

```hcl
resource "aws_api_gateway_rest_api" "api" {
  name        = var.project_name
  description = "Recipe to Reminders API"

  binary_media_types = ["*/*"]

  endpoint_configuration {
    types = ["REGIONAL"]
  }
}

# Root method (handles requests to /)
resource "aws_api_gateway_method" "root" {
  rest_api_id      = aws_api_gateway_rest_api.api.id
  resource_id      = aws_api_gateway_rest_api.api.root_resource_id
  http_method      = "ANY"
  authorization    = "NONE"
  api_key_required = true
}

resource "aws_api_gateway_integration" "root" {
  rest_api_id             = aws_api_gateway_rest_api.api.id
  resource_id             = aws_api_gateway_rest_api.api.root_resource_id
  http_method             = aws_api_gateway_method.root.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.api.invoke_arn
}

# Catch-all proxy resource
resource "aws_api_gateway_resource" "proxy" {
  rest_api_id = aws_api_gateway_rest_api.api.id
  parent_id   = aws_api_gateway_rest_api.api.root_resource_id
  path_part   = "{proxy+}"
}

resource "aws_api_gateway_method" "proxy" {
  rest_api_id      = aws_api_gateway_rest_api.api.id
  resource_id      = aws_api_gateway_resource.proxy.id
  http_method      = "ANY"
  authorization    = "NONE"
  api_key_required = true
}

resource "aws_api_gateway_integration" "proxy" {
  rest_api_id             = aws_api_gateway_rest_api.api.id
  resource_id             = aws_api_gateway_resource.proxy.id
  http_method             = aws_api_gateway_method.proxy.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.api.invoke_arn
}

# Deployment — redeploy when API config changes
resource "aws_api_gateway_deployment" "api" {
  rest_api_id = aws_api_gateway_rest_api.api.id

  triggers = {
    redeployment = sha1(jsonencode([
      aws_api_gateway_resource.proxy.id,
      aws_api_gateway_method.root.id,
      aws_api_gateway_method.proxy.id,
      aws_api_gateway_integration.root.id,
      aws_api_gateway_integration.proxy.id,
    ]))
  }

  lifecycle {
    create_before_destroy = true
  }
}

# Stage
resource "aws_api_gateway_stage" "prod" {
  deployment_id = aws_api_gateway_deployment.api.id
  rest_api_id   = aws_api_gateway_rest_api.api.id
  stage_name    = "prod"
}
```

**Step 2: Validate**

Run: `cd terraform && terraform validate`
Expected: `Success! The configuration is valid.`

**Step 3: Commit**

```bash
git add terraform/api-gateway.tf
git commit -m "feat: add API Gateway REST API with proxy integration"
```

---

### Task 9: Terraform — API Key and Rate Limiting

Create the API key, usage plan with throttle settings, and link them together.

**Files:**
- Create: `terraform/auth.tf`

**Step 1: Create `terraform/auth.tf`**

```hcl
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
```

**Step 2: Validate**

Run: `cd terraform && terraform validate`
Expected: `Success! The configuration is valid.`

**Step 3: Commit**

```bash
git add terraform/auth.tf
git commit -m "feat: add API key authentication and rate limiting"
```

---

### Task 10: Terraform — Outputs

Create outputs for the API URL, API key value, and bucket name.

**Files:**
- Create: `terraform/outputs.tf`

**Step 1: Create `terraform/outputs.tf`**

```hcl
output "api_url" {
  description = "API Gateway invoke URL"
  value       = "${aws_api_gateway_stage.prod.invoke_url}"
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
```

**Step 2: Validate all Terraform together**

Run: `cd terraform && terraform validate`
Expected: `Success! The configuration is valid.`

**Step 3: Commit**

```bash
git add terraform/outputs.tf
git commit -m "feat: add Terraform outputs for API URL, key, and bucket"
```

---

### Task 11: TESSDATA_PREFIX Environment Variable

The Go code in `tesseract.go` calls `tesseract stdin stdout -l eng --psm 6`. Tesseract needs to find `eng.traineddata` at runtime. The Lambda layer extracts to `/opt/`, so the trained data will be at `/opt/share/tessdata/eng.traineddata`. Tesseract uses the `TESSDATA_PREFIX` environment variable to locate this directory. This is already set in `lambda.tf` (`TESSDATA_PREFIX = "/opt/share/tessdata"`), but verify that the Go code doesn't hardcode a different path.

**Files:**
- Review: `internal/parser/tesseract.go`

**Step 1: Verify tesseract.go does NOT set TESSDATA_PREFIX**

Read `internal/parser/tesseract.go` and confirm:
- No `cmd.Env` assignment that would override `TESSDATA_PREFIX`
- No hardcoded tessdata path
- Tesseract inherits the Lambda environment variables by default via `os/exec`

The current code at lines 59-60:
```go
cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout",
    "-l", t.lang, "--psm", t.psm)
```

This correctly inherits the process environment (including `TESSDATA_PREFIX`). No code changes needed.

**Step 2: Verify Tesseract binary will be found**

The Lambda layer extracts to `/opt/`. The binary will be at `/opt/bin/tesseract`. Lambda's `PATH` includes `/opt/bin`, so `exec.CommandContext(ctx, "tesseract", ...)` will find it.

No code changes. This task is verification only.

**Step 3: Commit (no changes — verification task)**

No commit needed.

---

### Task 12: Build, Deploy, and Verify

Build all artifacts, deploy to AWS, and verify the deployment works end-to-end.

**Files:**
- None modified — this task uses the Makefile and Terraform from previous tasks

**Step 1: Build the Tesseract layer**

Run: `make layer`
Expected: `layers/tesseract/tesseract-layer.zip` created (~15-20MB). Takes 10-15 min on x86 hosts.

**Step 2: Build the Go binary**

Run: `make build`
Expected: `bootstrap` (ARM64 ELF) and `bootstrap.zip` created.

**Step 3: Create `terraform/terraform.tfvars`**

Copy `terraform/terraform.tfvars.example` to `terraform/terraform.tfvars` and fill in values:

```hcl
aws_region        = "ca-west-1"
anthropic_api_key = "sk-ant-..."  # Your actual key, or "" to disable
```

**Step 4: Deploy**

Run: `make deploy`

Terraform will show a plan. Review and type `yes`.

Expected resources created:
- S3 bucket with versioning, encryption, public access block, HTTPS-only policy
- IAM role and policies
- Lambda function with 2 layers
- API Gateway REST API, proxy resource, methods, integration, deployment, stage
- API key, usage plan, usage plan key

**Step 5: Retrieve the API key**

Run: `cd terraform && terraform output -raw api_key`
Save this value — you'll need it for testing and the iOS Shortcut.

**Step 6: Test the deployment**

Get the API URL:
Run: `cd terraform && terraform output -raw api_url`

Test with curl (replace `<URL>` and `<API_KEY>`):

```bash
# Health check — should return 405 (POST only on /extract)
curl -s -o /dev/null -w "%{http_code}" -H "x-api-key: <API_KEY>" <URL>/extract

# Extract from URL
curl -s -X POST -H "x-api-key: <API_KEY>" -H "Content-Type: application/json" \
  -d '{"url":"https://www.allrecipes.com/recipe/212721/grilled-salmon-i/"}' \
  <URL>/extract

# List recipes (should be empty)
curl -s -H "x-api-key: <API_KEY>" <URL>/recipes

# Verify API key is required (should return 403)
curl -s -o /dev/null -w "%{http_code}" <URL>/recipes
```

Expected:
- GET /extract: `405`
- POST /extract with URL: `200` with JSON ingredient list
- GET /recipes: `200` with `{"recipes":[]}`
- GET /recipes without key: `403`

**Step 7: Commit and push**

```bash
git add -A
git commit -m "docs: add terraform.tfvars.example"
git push origin main
```

Note: No new files should be staged (everything was committed in previous tasks). This push gets CI to verify the Go code still passes.

---

### Task 13: Verify CI

After pushing, verify that all CI jobs still pass.

**Step 1: Check CI status**

Run: `gh run list --limit 1`
Expected: All jobs pass (test, lint, gosec, govulncheck). The Terraform files and Makefile don't affect Go CI.

**Step 2: Close the milestone issue**

Run:
```bash
gh issue close 4 --comment "Milestone 4 complete. Deployed to AWS Lambda (ARM64) in ca-west-1 with API Gateway REST API, API key auth, Tesseract Lambda layer, S3 storage with versioning/encryption."
```
