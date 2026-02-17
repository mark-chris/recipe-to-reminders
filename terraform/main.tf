terraform {
  required_version = ">= 1.5"

  # Backend: Local state file. For durability, consider an S3 backend.
  # backend "s3" {
  #   bucket = "your-terraform-state-bucket"
  #   key    = "recipe-to-reminders/terraform.tfstate"
  #   region = "ca-west-1"
  # }

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

data "aws_caller_identity" "current" {}
