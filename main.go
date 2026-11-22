package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/litebitus/terraform-provider-awsextras/internal/awsextrasprovider"
)

var version = "dev"

func main() {
	err := providerserver.Serve(context.Background(), awsextrasprovider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/litebitus/awsextras",
	})
	if err != nil {
		log.Fatal(err)
	}
}
