package genvars

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver"

	"vpc-import-cli/common"
)

func Genvars(cfn_client_p *cloudformation.Client, ec2_client_p *ec2.Client, route53resolver_client_p *route53resolver.Client, stackName_p *string) {
	stackResourcesOutput_p := common.GetStackResourcesOutput(cfn_client_p, stackName_p)
	stacksOutput_p := common.GetStacksOutput(cfn_client_p, stackName_p)

	tfvars := initTfVarsFromStackParams(*stacksOutput_p)
	tfvars = mapTagsToTfvars(*stacksOutput_p, tfvars)
	tfvars = mapResolverRuleDetailsToTfvars(*stackResourcesOutput_p, route53resolver_client_p, tfvars)
	tfvars = mapDhcpOptionsToTfvars(*stackResourcesOutput_p, tfvars, ec2_client_p)
	tfvars = mapTgwAttachmentDetailsToTfvars(*stackResourcesOutput_p, tfvars, ec2_client_p)
	
	writeTfvarsToFile(tfvars)
}

type TfVars struct {
	Tags                          map[string]string `json:"tags"`
	SubnetCidrBits                string            `json:"SubnetCidrBits"`
	OrganizationId                string            `json:"OrganizationId"`
	DomainNameServers             []string          `json:"DomainNameServers"`
	DomainName                    string            `json:"DomainName"`
	IpRange                       string            `json:"IpRange`
	MasterAccountId               string            `json:"MasterAccountId"`
	SharedEnvironment             string            `json:"environment"`
	TransitGatewayID              string            `json:"TransitGatewayID"`
	TgwRouteTableID               string            `json:"TgwRouteTableID"`
	TgwMSKRouteTableID            string            `json:"TgwMSKRouteTableID"`
	VpcShareOU                    string            `json:"VpcShareOU"`
	DhcpOptions                   string            `json:"dhcp_options"`
	InternetResolverRuleId        string            `json:"internet_resolver_rule_id"`
	CrossVpcResolverRuleId        string            `json:"cross_vpc_resolver_rule_id"`
	MskccTldResolverRuleId        string            `json:"mskcc_tld_resolver_rule_id"`
	InternetResolverRuleAssocName string            `json:"internet_resolver_rule_assoc_name"`
	CrossVPCResolverRuleAssocName string            `json:"cross_vpc_resolver_rule_assoc_name"`
	MskccTldResolverRuleAssocName string            `json:"mskcc_tld_resolver_rule_assoc_name"`
	TgwAttachmentDnsSupport       string            `json:"tgw_attachment_dns_support"`
}

func getSubnetDetails(ec2_client_p *ec2.Client, physicalResourceId string) (string, string) {
	input := ec2.DescribeSubnetsInput{SubnetIds: []string{physicalResourceId}}
	output, err := ec2_client_p.DescribeSubnets(context.TODO(), &input)
	common.Check(err)
	return *output.Subnets[0].CidrBlock, *output.Subnets[0].AvailabilityZone
}

func getVpcRange(ec2_client_p *ec2.Client, physicalResourceId string) string {
	input := ec2.DescribeVpcsInput{VpcIds: []string{physicalResourceId}}
	output, err := ec2_client_p.DescribeVpcs(context.TODO(), &input)
	common.Check(err)
	return *output.Vpcs[0].CidrBlock
}

func getPhysicalResourceIdByLogicalResourceId(stackResourcesOutput cloudformation.DescribeStackResourcesOutput, targetLogicalResourceId string) string {
	for _, resource := range stackResourcesOutput.StackResources {
		physicalResourceId := *resource.PhysicalResourceId
		logicalResourceId := *resource.LogicalResourceId

		switch logicalResourceId {
		case targetLogicalResourceId:
			return physicalResourceId
		}
	}
	return ""
}

func mapDhcpOptionsToTfvars(stackResourcesOutput_p cloudformation.DescribeStackResourcesOutput, tfvars TfVars, ec2_client_p *ec2.Client) TfVars {
	for _, resource := range stackResourcesOutput_p.StackResources {
		physicalResourceId := *resource.PhysicalResourceId
		logicalResourceId := *resource.LogicalResourceId

		switch logicalResourceId {
		case "VPC":
			tfvars.DhcpOptions = common.GetDhcpOptionsIdFromVpc(ec2_client_p, physicalResourceId)
		}
	}
	return tfvars
}

func mapTgwAttachmentDetailsToTfvars(stackResourcesOutput_p cloudformation.DescribeStackResourcesOutput, tfvars TfVars, ec2_client_p *ec2.Client) TfVars {
	tgwAttachmentId := getPhysicalResourceIdByLogicalResourceId(stackResourcesOutput_p, "TgwAttach")
	filterName := "transit-gateway-attachment-id"
	input := ec2.DescribeTransitGatewayVpcAttachmentsInput{Filters: []ec2_types.Filter{{Name: &filterName, Values: []string{tgwAttachmentId}}}}
	tgwAttachmentsOutput, err := ec2_client_p.DescribeTransitGatewayVpcAttachments(context.TODO(), &input)
	common.Check(err)
	dnsSupport := tgwAttachmentsOutput.TransitGatewayVpcAttachments[0].Options.DnsSupport
	tfvars.TgwAttachmentDnsSupport = getStringFromDnsSupportEnum(dnsSupport)
	return tfvars
}

func getStringFromDnsSupportEnum(dnsSupportEnum ec2_types.DnsSupportValue) string {
	switch dnsSupportEnum {
	case ec2_types.DnsSupportValueEnable:
		return "enable"
	case ec2_types.DnsSupportValueDisable:
		return "disable"
	}
	return "unknown"
}

func mapTagsToTfvars(stacksOutput cloudformation.DescribeStacksOutput, tfvars TfVars) TfVars {
	tfvars.Tags = map[string]string{}
	for _, tag := range stacksOutput.Stacks[0].Tags {
		tfvars.Tags[*tag.Key] = *tag.Value
	}
	return tfvars
}

func initTfVarsFromStackParams(stacksOutput cloudformation.DescribeStacksOutput) TfVars {
	params := map[string]string{}
	for _, param := range stacksOutput.Stacks[0].Parameters {
		if param.ResolvedValue != nil {
			params[*param.ParameterKey] = *param.ResolvedValue
		} else {
			params[*param.ParameterKey] = *param.ParameterValue
		}
	}
	return TfVars{
		SubnetCidrBits: params["SubnetCidrBits"],
		OrganizationId: params["OrganizationId"],
		DomainNameServers: []string{params["DomainNameServers"]},
		DomainName: params["DomainName"],
		IpRange: params["IpRange"],
		MasterAccountId: params["MasterAccountId"],
		SharedEnvironment: params["SharedEnvironment"],
		TransitGatewayID: params["TransitGatewayID"],
		TgwRouteTableID: params["TgwRouteTableID"],
		TgwMSKRouteTableID: params["TgwMSKRouteTableID"],
		VpcShareOU: params["VpcShareOU"],
		DhcpOptions: params["DhcpOptions"],
		InternetResolverRuleId: params["InternetResolverRuleId"],
		CrossVpcResolverRuleId: params["CrossVpcResolverRuleId"],
		MskccTldResolverRuleId: params["MskccTldResolverRuleId"],
		InternetResolverRuleAssocName: params["InternetResolverRuleAssocName"],
		CrossVPCResolverRuleAssocName: params["CrossVPCResolverRuleAssocName"],
		MskccTldResolverRuleAssocName: params["MskccTldResolverRuleAssocName"],
		TgwAttachmentDnsSupport: params["TgwAttachmentDnsSupport"],
	}
}

func mapResolverRuleDetailsToTfvars(stackResourcesOutput_p cloudformation.DescribeStackResourcesOutput, client *route53resolver.Client, tfvars TfVars) TfVars {
	vpcId := getPhysicalResourceIdByLogicalResourceId(stackResourcesOutput_p, "VPC")
	internetAssoc := common.GetResolverRuleAssociation(client, vpcId, common.ResolverRuleInternet)
	tldAssoc := common.GetResolverRuleAssociation(client, vpcId, common.ResolverRuleTldIac)

	if tldAssoc == nil {
		tldAssoc = common.GetResolverRuleAssociation(client, vpcId, common.ResolverRuleTldLegacy)
	}

	crossVpcAssoc := common.GetResolverRuleAssociation(client, vpcId, common.ResolverRuleCrossVpcIac)

	if crossVpcAssoc == nil {
		crossVpcAssoc = common.GetResolverRuleAssociation(client, vpcId, common.ResolverRuleCrossVpcLegacy)
	}

	if internetAssoc != nil {
		tfvars.InternetResolverRuleAssocName = *internetAssoc.Name
		tfvars.InternetResolverRuleId = *internetAssoc.ResolverRuleId
	}

	if crossVpcAssoc != nil {
		tfvars.CrossVPCResolverRuleAssocName = *crossVpcAssoc.Name
		tfvars.CrossVpcResolverRuleId = *crossVpcAssoc.ResolverRuleId
	}

	if tldAssoc != nil {
		tfvars.MskccTldResolverRuleAssocName = *tldAssoc.Name
		tfvars.MskccTldResolverRuleId = *tldAssoc.ResolverRuleId
	}

	return tfvars
}

func writeTfvarsToFile(tfvars TfVars) {
	var out *bytes.Buffer = bytes.NewBuffer(make([]byte, 0, 4096))
	var err error
	var tfvarsJson []byte
	var name string
	var path string
	var f *os.File

	defer f.Close()

	tfvarsJson, err = json.Marshal(tfvars)
	common.Check(err)
	json.Indent(out, tfvarsJson, "", "    ")
	path, err = os.Getwd()
	common.Check(err)
	name = path + "/terraform.tfvars.json"

	if _, err = os.Stat(name); err == nil {
		err = errors.New("cli.go: writeTfvarsToFile(tfvars TfVars): tfvars file already exists: " + name)
		log.Fatal(err)
	}

	f, err = os.Create(name) // Note: This operation truncates an existing file
	common.Check(err)
	fmt.Fprintln(os.Stderr, "Writing JSON for generated tfvars to Path: "+f.Name())
	w := bufio.NewWriter(f)
	out.WriteTo(w)
	w.Flush()
}
