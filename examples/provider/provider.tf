terraform {
  required_providers {
    awsextras = {
      source = "litebitus/awsextras"
    }
  }
}

provider "awsextras" {
  region = "us-west-2"

  assume_role {
    role_arn     = "arn:aws:iam::000000000000:role/mock-role"
    session_name = "mock-session"
  }

  default_tags {
    tags = { owner = "mock-team" }
  }
}
