# Milestone 4: AWS Deployment — Design

**Goal:** Deploy the Recipe to Reminders backend to AWS Lambda behind API Gateway with Tesseract OCR support, S3 storage, API key authentication, and rate limiting.

**Architecture:**

```
iOS Shortcut → API Gateway (REST API v1) → Lambda (ARM64, 2048MB)
                                              ├── Lambda Web Adapter layer
                                              ├── Tesseract layer (OCR binary + tessdata)
                                              ├── Go binary (bootstrap)
                                              └── S3 (recipes.json, versioned, encrypted)
```

## Lambda Function

- **Runtime:** `provided.al2023` (custom runtime, Go binary as `bootstrap`)
- **Architecture:** `arm64` (cost-efficient for compute-bound OCR)
- **Memory:** 2048 MB (Tesseract OCR on large images)
- **Timeout:** 60s (Tesseract + Claude fallback + S3 retries)
- **Reserved concurrency:** 10 (prevent request floods)
- **Layers:** Lambda Web Adapter (AWS-published ARN for ARM64) + custom Tesseract layer

### Lambda Web Adapter

The AWS Lambda Web Adapter (`arn:aws:lambda:{region}:753240598075:layer:LambdaAdapterLayerArm64:{version}`) translates API Gateway events into HTTP requests and proxies them to the Go HTTP server. Zero code changes to `cmd/lambda/main.go`.

Environment variables:
- `AWS_LAMBDA_EXEC_WRAPPER=/opt/bootstrap` (activates the adapter)
- `PORT=8080` (matches the Go server's default port)

### Environment Variables (via Terraform)

- `S3_BUCKET`, `S3_RECIPES_KEY` — from Terraform S3 resource
- `TESSERACT_PSM=6`, `TESSERACT_LANG=eng`
- `CONFIDENCE_THRESHOLD=0.65`
- `PHOTO_STRATEGY=tesseract-first`
- `ANTHROPIC_API_KEY` — from `terraform.tfvars` (sensitive, not committed)

### Deployment Artifact

A zip containing the `bootstrap` binary:
```
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/lambda/
```

## API Gateway (REST API v1)

REST API v1 instead of HTTP API v2. The CLAUDE.md spec requires API key authentication via `x-api-key` header with usage plans and per-route throttling — these are REST API features not available in HTTP API. At personal scale, the cost difference ($3.50/million vs $1.00/million) is irrelevant.

### Structure

Single REST API with one stage (`prod`) and a catch-all `{proxy+}` resource forwarding everything to Lambda. The Go server's `http.ServeMux` handles routing.

### Authentication

- One API key created in Terraform
- One usage plan linking the key to the `prod` stage
- `api_key_required = true` on the method
- Key value output as a sensitive Terraform output for the iOS Shortcut

### Rate Limiting

Usage plan throttle settings:
- Burst: 10 req/sec
- Sustained: 5 req/sec

Per-route overrides deferred — global limits are sufficient at personal scale.

### Payload & Media Types

- REST API supports up to 10MB request body (accommodates 7MB image limit, ~9.3MB after base64)
- `*/*` configured as binary media type for image payloads

## S3 Bucket

- **Versioning enabled** with lifecycle rule expiring noncurrent versions after 90 days
- **Block public access** — all four settings enabled
- **Enforce HTTPS** — bucket policy denying `aws:SecureTransport = false`
- **Encryption at rest** — SSE-S3 (AES-256) default encryption
- **IAM scoping** — Lambda role gets `s3:GetObject`, `s3:PutObject` on `recipes.json` only, plus `s3:GetObjectVersion` and `s3:ListBucketVersions` for corruption recovery
- **CloudWatch Logs** — Lambda role gets `logs:CreateLogGroup`, `logs:CreateLogStream`, `logs:PutLogEvents` scoped to its log group

## Tesseract Lambda Layer

Docker-based build script (`layers/tesseract/build.sh`) compiling Tesseract 5.x and Leptonica for Amazon Linux 2023 ARM64:

1. `amazonlinux:2023` Docker image with `--platform linux/arm64` (QEMU on x86 hosts)
2. Build Leptonica from source (static linking)
3. Build Tesseract 5.x from source (static linking against Leptonica)
4. Download `eng.traineddata` from tessdata_best
5. Package into layer zip:
   ```
   bin/tesseract
   share/tessdata/eng.traineddata
   ```
6. Output: `layers/tesseract/tesseract-layer.zip` (~15-20MB)

The zip is .gitignored (build artifact). Terraform references it as a local file.

## Terraform Structure

```
terraform/
├── main.tf           # Provider config, backend (local state)
├── variables.tf      # Input variables
├── lambda.tf         # Lambda function, layers, IAM role/policy
├── api-gateway.tf    # REST API, resource, method, integration, deployment, stage
├── auth.tf           # API key, usage plan, usage plan key
├── s3.tf             # Bucket, versioning, encryption, public access block, policy
├── outputs.tf        # API URL, API key value (sensitive), bucket name
└── terraform.tfvars  # User-specific values (NOT committed, .gitignored)
```

**State:** Local (`terraform.tfstate`). No remote state for a personal project.

### Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `aws_region` | string | `ca-west-1` | AWS region (Canada West) |
| `project_name` | string | `recipe-to-reminders` | Resource naming prefix |
| `anthropic_api_key` | string (sensitive) | — | Claude API key |
| `lambda_memory` | number | `2048` | Lambda memory in MB |
| `lambda_timeout` | number | `60` | Lambda timeout in seconds |
| `lambda_reserved_concurrency` | number | `10` | Max concurrent executions |

## Build & Deploy

### Makefile

```
make build     # Cross-compile Go binary for ARM64, zip it
make layer     # Build Tesseract layer via Docker (~15 min first time)
make deploy    # terraform apply
make destroy   # terraform destroy
make clean     # Remove build artifacts
```

### No CI/CD Deployment

Deployment is manual via `make deploy`. Adding a GitHub Actions deployment pipeline would require storing AWS credentials in GitHub Secrets — unnecessary complexity for a personal tool. CI continues to run tests/lint/security only.

### .gitignore Additions

`bootstrap`, `bootstrap.zip`, `layers/tesseract/tesseract-layer.zip`, `terraform/.terraform/`, `terraform/*.tfstate*`, `terraform/terraform.tfvars`

## What's Not Included (YAGNI)

- CloudWatch alarms
- WAF
- Custom domain / Route 53
- CI/CD deployment pipeline
- Remote Terraform state
- Multiple environments (dev/staging/prod)
