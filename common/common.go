package common

import (
	"context"
	"log"

	cfn "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfn_types "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver"
	route53resolver_types "github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
)

// const:
const (
	ResolverRuleTldLegacy      = "MSKCC TLD"
	ResolverRuleTldIac         = "hccp-mskcc-tld-rule"
	ResolverRuleCrossVpcLegacy = "AWS subdomain for cross VPC resolution"
	ResolverRuleCrossVpcIac    = "hccp-cross-vpc-rule"
	ResolverRuleInternet       = "Internet Resolver"
)

func GetStacksOutput(cfn_client_p *cfn.Client, stackName_p *string) *cfn.DescribeStacksOutput {
	stacksInput := cfn.DescribeStacksInput{StackName: stackName_p}
	stacksOutput_p, err := cfn_client_p.DescribeStacks(context.TODO(), &stacksInput)
	Check(err)
	return stacksOutput_p
}

func GetStackResourcesOutput(cfn_client_p *cfn.Client, stackName_p *string) *cfn.DescribeStackResourcesOutput {
	stackResourcesInput := cfn.DescribeStackResourcesInput{StackName: stackName_p}
	stackResourcesOutput, err := cfn_client_p.DescribeStackResources(context.TODO(), &stackResourcesInput)
	Check(err)
	return stackResourcesOutput
}

func GetDhcpOptionsIdFromVpc(ec2_client_p *ec2.Client, physicalResourceId string) string {
	input := ec2.DescribeVpcsInput{VpcIds: []string{physicalResourceId}}
	output, err := ec2_client_p.DescribeVpcs(context.TODO(), &input)
	Check(err)
	return *output.Vpcs[0].DhcpOptionsId
}

func GetResolverRuleAssociation(route53resolver_client_p *route53resolver.Client, vpcId string, resolver_rule_name string) *route53resolver_types.ResolverRuleAssociation {
	resolverRuleId := getResolverRuleId(route53resolver_client_p, resolver_rule_name)
	resolverRuleIdFilterName := "ResolverRuleId"
	vpcIdFilterName := "VPCId"
	filters := []route53resolver_types.Filter{
		{
			Name:   &vpcIdFilterName,
			Values: []string{vpcId},
		},
		{
			Name:   &resolverRuleIdFilterName,
			Values: []string{resolverRuleId},
		},
	}
	input := route53resolver.ListResolverRuleAssociationsInput{Filters: filters}
	output, err := route53resolver_client_p.ListResolverRuleAssociations(context.TODO(), &input)
	Check(err)

	if len(output.ResolverRuleAssociations) > 0 {
		rule := output.ResolverRuleAssociations[0]
		return &rule
	}

	return nil
}

func getResolverRuleId(route53resolver_client_p *route53resolver.Client, resolver_rule_name string) string {
	nameFilterName := "Name"
	filters := []route53resolver_types.Filter{
		{
			Name:   &nameFilterName,
			Values: []string{resolver_rule_name},
		},
	}
	input := route53resolver.ListResolverRulesInput{Filters: filters}
	output, err := route53resolver_client_p.ListResolverRules(context.TODO(), &input)
	Check(err)
	return *output.ResolverRules[0].Id
}

func Check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

// generic Filter function type
type filterFunc[A any] func(A) bool

// generic Filter function that takes instance of filterFunc type as a param
func Filter[A any](input []A, f filterFunc[A]) []A {
	var output []A
	for _, element := range input {
		if f(element) {
			output = append(output, element)
		}
	}
	return output
}

func GetParameterValue(stackResourcesOutput_p cfn.DescribeStacksOutput,
	paramKey string) string {
	return *GetParameter(stackResourcesOutput_p, paramKey).ParameterValue
}

func GetParameterResolvedValue(stackResourcesOutput_p cfn.DescribeStacksOutput,
	paramKey string) string {
	return *GetParameter(stackResourcesOutput_p, paramKey).ResolvedValue
}

func GetParameter(stacksOutput_p cfn.DescribeStacksOutput,
	paramKey string) cfn_types.Parameter {
	params := stacksOutput_p.Stacks[0].Parameters
	f := func(param cfn_types.Parameter) bool {
		return *param.ParameterKey == paramKey
	}
	return Filter(params, f)[0]
}
