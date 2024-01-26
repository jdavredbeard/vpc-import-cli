## vpc-import-cli  
  
`vpc-import-cli` is a helper cli that imports a given VPC stack from AWS CloudFormation into Terraform, and generates a `tfvars` file for it, intended to parameterize the Terraform config.  
  
The tool works well for a cloudformation stack of a specific shape, but Terraform import is a difficult process to fully generalize because the ID that Terraform uses to direct its import of a given resource type is frequently a concatenated string of several related resources' IDs or attributes, so each resource type would need a custom function to pull the required metadata from AWS in order to import that type. Perhaps the code could be generated based on the Terraform codebase.  

Standard `go build` and `go run .` commands work here - also see Makefile for other tasks.
