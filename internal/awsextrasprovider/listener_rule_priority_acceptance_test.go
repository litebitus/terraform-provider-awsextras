package awsextrasprovider

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// acceptanceListener is a listener on a load balancer in the AWS API the
// environment points at, LocalStack in `make test-acceptance`.
type acceptanceListener struct {
	client      *elbv2.Client
	listenerARN string
}

// newAcceptanceListener makes a load balancer with one listener and returns it;
// the test's cleanup deletes what it made, whether the test passed or failed.
func newAcceptanceListener(t *testing.T) *acceptanceListener {
	t.Helper()
	ctx := context.Background()
	sdkConfig, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	network := ec2.NewFromConfig(sdkConfig)
	vpc, err := network.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/16")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = network.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: vpc.Vpc.VpcId}) })
	var subnetIDs []string
	for index, zone := range []string{"a", "b"} {
		subnet, err := network.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            vpc.Vpc.VpcId,
			CidrBlock:        aws.String([]string{"10.0.1.0/24", "10.0.2.0/24"}[index]),
			AvailabilityZone: aws.String(sdkConfig.Region + zone),
		})
		if err != nil {
			t.Fatal(err)
		}
		subnetID := aws.ToString(subnet.Subnet.SubnetId)
		subnetIDs = append(subnetIDs, subnetID)
		t.Cleanup(func() { _, _ = network.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subnetID)}) })
	}
	client := elbv2.NewFromConfig(sdkConfig)
	balancer, err := client.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		Name:    aws.String("mock-lb"),
		Subnets: subnetIDs,
		Scheme:  elbv2types.LoadBalancerSchemeEnumInternal,
		Type:    elbv2types.LoadBalancerTypeEnumApplication,
	})
	if err != nil {
		t.Fatal(err)
	}
	balancerARN := balancer.LoadBalancers[0].LoadBalancerArn
	t.Cleanup(func() {
		_, _ = client.DeleteLoadBalancer(ctx, &elbv2.DeleteLoadBalancerInput{LoadBalancerArn: balancerARN})
	})
	listener, err := client.CreateListener(ctx, &elbv2.CreateListenerInput{
		LoadBalancerArn: balancerARN,
		Port:            aws.Int32(80),
		Protocol:        elbv2types.ProtocolEnumHttp,
		DefaultActions: []elbv2types.Action{{
			Type:                elbv2types.ActionTypeEnumFixedResponse,
			FixedResponseConfig: &elbv2types.FixedResponseActionConfig{StatusCode: aws.String("404")},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &acceptanceListener{client: client, listenerARN: aws.ToString(listener.Listeners[0].ListenerArn)}
}

// addRule adds a rule at priority with tags, as a caller of the resource would.
func (listener *acceptanceListener) addRule(t *testing.T, priority int32, path string, tags map[string]string) {
	t.Helper()
	var ruleTags []elbv2types.Tag
	for key, value := range tags {
		ruleTags = append(ruleTags, elbv2types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	_, err := listener.client.CreateRule(context.Background(), &elbv2.CreateRuleInput{
		ListenerArn: aws.String(listener.listenerARN),
		Priority:    aws.Int32(priority),
		Conditions: []elbv2types.RuleCondition{{
			Field:  aws.String("path-pattern"),
			Values: []string{path},
		}},
		Actions: []elbv2types.Action{{
			Type:                elbv2types.ActionTypeEnumFixedResponse,
			FixedResponseConfig: &elbv2types.FixedResponseActionConfig{StatusCode: aws.String("200")},
		}},
		Tags: ruleTags,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceListenerRulePriorityIsAllocatedFromTheListenersRulesAndKept(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("acceptance test: run with make test-acceptance")
	}
	listener := newAcceptanceListener(t)
	listener.addRule(t, 1000, "/mock-other", map[string]string{"mock-owner": "mock-other"})
	config := `
provider "awsextras" {
  default_tags {
    tags = { mock-owner = "mock-a" }
  }
}

resource "awsextras_lb_listener_rule_priority" "mock" {
  listener_arn = "` + listener.listenerARN + `"
  min_priority = 1000
  max_priority = 1010
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protocolProviders,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("awsextras_lb_listener_rule_priority.mock", tfjsonpath.New("priority"),
						knownvalue.Int64Exact(1001)),
				},
			},
			{
				PreConfig: func() {
					listener.addRule(t, 1001, "/mock-a", map[string]string{priorityTagKey: "1001", "mock-owner": "mock-a"})
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
