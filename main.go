package main

import (
	"context"
	"errors"
	"flag"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver"

	"vpc-import-cli/common"
	"vpc-import-cli/genvars"
	"vpc-import-cli/tf_import"
)

func main() {
	// new() initializes the var and returns a pointer to it, so
	// for ex. stackName_p is a pointer
	stackName_p := new(string)
	import_p := new(bool)
	genvars_p := new(bool)
	// the flag methods takes a pointer to the var which will hold the
	// value from the command line
	flag.StringVar(stackName_p, "stack-name", "", "The StackName of the networking-dedicated-spoke stack to import")
	flag.BoolVar(import_p, "import", false, "Boolean flag, set to import stack with name passed to --stack-name")
	flag.BoolVar(genvars_p, "genvars", false, "Boolean flag, set to generate tfvars file for stack with name passed to --stack-name")
	flag.Parse()
	// *stackName_p is the value pointed to by stackName_p
	if *stackName_p == "" {
		log.Fatal(errors.New("value for '--stack-name' flag is required"))
	}
	if !*genvars_p && !*import_p {
		log.Fatal(errors.New("either --genvars or --import is required"))
	}
	// Load the Shared AWS Configuration (~/.aws/config)
	cfg, err := config.LoadDefaultConfig(context.TODO())
	common.Check(err)

	// these methods also return points to the clients
	cfn_client_p := cloudformation.NewFromConfig(cfg)
	ec2_client_p := ec2.NewFromConfig(cfg)
	route53resolver_client_p := route53resolver.NewFromConfig(cfg)

	if *genvars_p {
		genvars.Genvars(cfn_client_p, ec2_client_p, route53resolver_client_p, stackName_p)
	}
	if *import_p {
		tf_import.TerraformImport(cfn_client_p, ec2_client_p, route53resolver_client_p, stackName_p)
	}
}
