package tf_import

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/terraform-exec/tfexec"

	"vpc-import-cli/common"
)

// mapping of cloudformation logical ids to terraform resource ids
var logicalIdToTfResourceId = map[string]string{
	"VPC":                         "module.vpc.aws_vpc.main",
	"DhcpOptions":                 "module.vpc.aws_vpc_dhcp_options.main",
	"DefaultNacl":                 "module.vpc.aws_default_network_acl.main",
	"RouteTable":                  "module.vpc.aws_route_table.main",
	"Route":                       "module.vpc.aws_route.main",
	"SgBase":                      "module.vpc.aws_security_group.base",
	"SgBaseEgress":                "module.vpc.aws_security_group_rule.base_egress",
	"SgBaseIngressV4":             "module.vpc.aws_security_group_rule.base_ingress_v4",
	"Subnet1":                     "module.vpc.aws_subnet.subnet_1",
	"Subnet2":                     "module.vpc.aws_subnet.subnet_2",
	"Subnet3":                     "module.vpc.aws_subnet.subnet_3",
	"SubnetRouteAssociation1":     "module.vpc.aws_route_table_association.rt_association_1",
	"SubnetRouteAssociation2":     "module.vpc.aws_route_table_association.rt_association_2",
	"SubnetRouteAssociation3":     "module.vpc.aws_route_table_association.rt_association_3",
	"TgwRoute":                    "module.vpc.aws_ec2_transit_gateway_route.main",
	"TgwAttach":                   "module.vpc.aws_ec2_transit_gateway_vpc_attachment.main[\"0\"]",
	"TgwRouteAssocation":          "module.vpc.aws_ec2_transit_gateway_route_table_association.main",
	"TgwRoutePropagation":         "module.vpc.aws_ec2_transit_gateway_route_table_propagation.main",
	"TgwMSKAttachmentPropagation": "module.vpc.aws_ec2_transit_gateway_route_table_propagation.msk",
	"ResourceShare":               "module.vpc.aws_ram_resource_share.vpc",
	"VpcDhcp":                     "module.vpc.aws_vpc_dhcp_options_association.main",
	"VpcEndpointEC2":              "module.vpc.aws_vpc_endpoint.ec2",
	"VpcEndpointEC2Messages":      "module.vpc.aws_vpc_endpoint.ec2messages",
	"VpcEndpointS3":               "module.vpc.aws_vpc_endpoint.s3",
	"VpcEndpointSSM":              "module.vpc.aws_vpc_endpoint.ssm",
	"FlowLogsVpcEnable":           "module.vpc.aws_flow_log.main",
}

func TerraformImport(cfn_client_p *cloudformation.Client,
	ec2_client_p *ec2.Client,
	route53resolver_client_p *route53resolver.Client,
	stackName_p *string) {

	stackResourcesOutput_p := common.GetStackResourcesOutput(cfn_client_p, stackName_p)
	stacksOutput_p := common.GetStacksOutput(cfn_client_p, stackName_p)

	tfResourceIdsToPhysicalIds := mapTfResourceIdsToPhysicalIds(ec2_client_p,
		route53resolver_client_p,
		*stacksOutput_p,
		*stackResourcesOutput_p)

	tf := terraformInit()

	for tfResourceId, physicalId := range tfResourceIdsToPhysicalIds {
		log.Printf("Importing PhysicalId: %s to Resource Address: %s", physicalId, tfResourceId)
		err := tf.Import(context.Background(), tfResourceId, physicalId)
		common.Check(err)
	}
}

func mapTfResourceIdsToPhysicalIds(ec2_client_p *ec2.Client,
	route53resolver_client_p *route53resolver.Client,
	stacksOutput_p cloudformation.DescribeStacksOutput,
	stackResourcesOutput_p cloudformation.DescribeStackResourcesOutput) map[string]string {

	vpc_ip_range := common.GetParameterValue(stacksOutput_p, "IpRange")
	tgw_route_table_id := common.GetParameterResolvedValue(stacksOutput_p, "TgwRouteTableID")
	msk_tgw_route_table_id := common.GetParameterResolvedValue(stacksOutput_p, "TgwMSKRouteTableID")

	logicalIdsToPhysicalIds := map[string]string{}

	for _, resource := range stackResourcesOutput_p.StackResources {
		physicalResourceId := *resource.PhysicalResourceId
		logicalResourceId := *resource.LogicalResourceId
		if logicalIdToTfResourceId[logicalResourceId] != "" {
			logicalIdsToPhysicalIds[logicalResourceId] = physicalResourceId
		}
	}

	// update these physical ids to match format required by terraform import
	logicalIdsToPhysicalIds["SubnetRouteAssociation1"] = logicalIdsToPhysicalIds["Subnet1"] + "/" + logicalIdsToPhysicalIds["RouteTable"]
	logicalIdsToPhysicalIds["SubnetRouteAssociation2"] = logicalIdsToPhysicalIds["Subnet2"] + "/" + logicalIdsToPhysicalIds["RouteTable"]
	logicalIdsToPhysicalIds["SubnetRouteAssociation3"] = logicalIdsToPhysicalIds["Subnet3"] + "/" + logicalIdsToPhysicalIds["RouteTable"]

	// update security group rule ids to match format required by terraform import
	ingressId, egressId := getSecurityGroupRulePhysicalIds(ec2_client_p, logicalIdsToPhysicalIds["SgBase"])
	logicalIdsToPhysicalIds["SgBaseIngressV4"] = ingressId
	logicalIdsToPhysicalIds["SgBaseEgress"] = egressId

	// update dhcp options id with that associated with the vpc rather than that defined in the stack (in case they are not the same)
	logicalIdsToPhysicalIds["DhcpOptions"] = common.GetDhcpOptionsIdFromVpc(ec2_client_p, logicalIdsToPhysicalIds["VPC"])
	logicalIdsToPhysicalIds["DefaultNacl"] = getDefaultNaclIdFromVpc(ec2_client_p, logicalIdsToPhysicalIds["VPC"])

	// update dhcp options association id with VPC id to match requirement for terraform import
	logicalIdsToPhysicalIds["VpcDhcp"] = logicalIdsToPhysicalIds["VPC"]

	// update default Route physical id to match requirement for terraform import
	logicalIdsToPhysicalIds["Route"] = logicalIdsToPhysicalIds["RouteTable"] + "_0.0.0.0/0"

	// update TgwRoute physical id to match requirement for terraform import
	logicalIdsToPhysicalIds["TgwRoute"] = tgw_route_table_id + "_" + vpc_ip_range

	// update TgwRouteAssocation physical id to match requirement for terraform import
	// see https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ec2_transit_gateway_route_table_association#import
	logicalIdsToPhysicalIds["TgwRouteAssocation"] = tgw_route_table_id + "_" + logicalIdsToPhysicalIds["TgwAttach"]

	// update TgwRoutePropagation physical id to match requirement for terraform import
	// yes, it has the exact same calculated id as TgwRouteAssocation, see
	// https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ec2_transit_gateway_route_table_propagation#import
	logicalIdsToPhysicalIds["TgwRoutePropagation"] = tgw_route_table_id + "_" + logicalIdsToPhysicalIds["TgwAttach"]

	// update TgwMSKAttachmentPropagation physical id to match requirement for terraform import
	logicalIdsToPhysicalIds["TgwMSKAttachmentPropagation"] = msk_tgw_route_table_id + "_" + logicalIdsToPhysicalIds["TgwAttach"]

	tfResourceIdsToPhysicalIds := map[string]string{}

	for logicalId, tfResourceId := range logicalIdToTfResourceId {
		// this logic will remove flow logs (and other resources) from list of resources to import if physical id doesn't exist
		// (in cloudformation stack flow log resource is only created in main org)
		// vpc module will create new flow log resource instead
		if logicalIdsToPhysicalIds[logicalId] != "" {
			tfResourceIdsToPhysicalIds[tfResourceId] = logicalIdsToPhysicalIds[logicalId]
		}
	}

	// get resolver rule association ids and add them to mapping to be imported - these association resources are not included
	// in the cloudformation stack but are necessary for the dns functionality of the vpc, and will be managed as part of the
	// vpc terraform module

	internetAssoc := common.GetResolverRuleAssociation(route53resolver_client_p, logicalIdsToPhysicalIds["VPC"], common.ResolverRuleInternet)

	tldAssoc := common.GetResolverRuleAssociation(route53resolver_client_p, logicalIdsToPhysicalIds["VPC"], common.ResolverRuleTldIac)

	if tldAssoc == nil {
		tldAssoc = common.GetResolverRuleAssociation(route53resolver_client_p, logicalIdsToPhysicalIds["VPC"], common.ResolverRuleTldLegacy)
	}

	crossVpcAssoc := common.GetResolverRuleAssociation(route53resolver_client_p, logicalIdsToPhysicalIds["VPC"], common.ResolverRuleCrossVpcIac)

	if crossVpcAssoc == nil {
		crossVpcAssoc = common.GetResolverRuleAssociation(route53resolver_client_p, logicalIdsToPhysicalIds["VPC"], common.ResolverRuleCrossVpcLegacy)
	}

	if internetAssoc != nil {
		tfResourceIdsToPhysicalIds["module.vpc.aws_route53_resolver_rule_association.internet"] = *internetAssoc.Id
	}

	if tldAssoc != nil {
		tfResourceIdsToPhysicalIds["module.vpc.aws_route53_resolver_rule_association.mskcc_tld"] = *tldAssoc.Id
	}

	if crossVpcAssoc != nil {
		tfResourceIdsToPhysicalIds["module.vpc.aws_route53_resolver_rule_association.cross_vpc"] = *crossVpcAssoc.Id
	}

	return tfResourceIdsToPhysicalIds
}

func terraformInit() *tfexec.Terraform {
	var required_version = "1.4.6"
	fsTfVersion := &fs.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion(required_version)),
	}

	log.Println("Finding existing terraform install for version: " + required_version)
	execPath, err := fsTfVersion.Find(context.Background())
	if err != nil {
		log.Fatalf("error finding Terraform: %s", err)
	}

	workingDir := "."
	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		log.Fatalf("error running NewTerraform: %s", err)
	}

	log.Println("Running terraform init...")
	err = tf.Init(context.Background(), tfexec.Upgrade(true))
	if err != nil {
		log.Fatalf("error running Init: %s", err)
	}

	return tf
}

func getSecurityGroupRulePhysicalIds(ec2_client_p *ec2.Client, physicalResourceId string) (string, string) {
	filterName := "group-id"
	input := ec2.DescribeSecurityGroupRulesInput{Filters: []ec2_types.Filter{{Name: &filterName, Values: []string{physicalResourceId}}}}
	output, err := ec2_client_p.DescribeSecurityGroupRules(context.TODO(), &input)
	common.Check(err)
	ingressId := ""
	egressId := ""
	for _, sg := range output.SecurityGroupRules {
		if !*sg.IsEgress {
			ingressId = formatSecurityGroupRuleId(sg, "ingress")
		} else {
			egressId = formatSecurityGroupRuleId(sg, "egress")
		}
	}
	return ingressId, egressId
}

func getDefaultNaclIdFromVpc(ec2_client_p *ec2.Client, physicalResourceId string) string {
	vpcIdStr := "vpc-id"
	defaultStr := "default"
	filters := []ec2_types.Filter{
		{
			Name:   &vpcIdStr,
			Values: []string{physicalResourceId},
		},
		{
			Name:   &defaultStr,
			Values: []string{"true"},
		},
	}
	input := ec2.DescribeNetworkAclsInput{Filters: filters}
	output, err := ec2_client_p.DescribeNetworkAcls(context.TODO(), &input)
	common.Check(err)
	return *output.NetworkAcls[0].NetworkAclId
}

func formatSecurityGroupRuleId(sg ec2_types.SecurityGroupRule, sg_rule_type string) string {
	var protocol string
	var fromPort string
	var toPort string
	if *sg.IpProtocol == "-1" {
		protocol = "all"
	} else {
		protocol = *sg.IpProtocol
	}
	if protocol == "all" && strconv.Itoa(int(*sg.FromPort)) == "-1" {
		fromPort = "0"
	} else {
		fromPort = strconv.Itoa(int(*sg.FromPort))
	}
	if protocol == "all" && strconv.Itoa(int(*sg.ToPort)) == "-1" {
		toPort = "0"
	} else {
		toPort = strconv.Itoa(int(*sg.ToPort))
	}
	return fmt.Sprintf("%s_%s_%s_%s_%s_%s", *sg.GroupId, sg_rule_type, protocol, fromPort, toPort, *sg.CidrIpv4)
}
