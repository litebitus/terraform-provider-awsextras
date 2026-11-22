package awsextrasprovider

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

var protocolProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"awsextras": providerserver.NewProtocol6WithError(New("test")()),
}

// withMockListener points every provider configuration at listener for the
// test, and back once it ends, whether it passed or failed.
func withMockListener(t *testing.T, listener *mockListener) {
	t.Helper()
	original := newListenerRules
	newListenerRules = func(aws.Config) listenerRules { return listener }
	t.Cleanup(func() { newListenerRules = original })
}

func allocationConfig(maxPriority int, tags string) string {
	return `
provider "awsextras" {
  region = "mock-region"
  default_tags {
    tags = { mock-owner = "mock-a" }
  }
}

resource "awsextras_lb_listener_rule_priority" "mock" {
  listener_arn = "` + mockListenerARN + `"
  min_priority = 1000
  max_priority = ` + strconv.Itoa(maxPriority) + `
  tags         = ` + tags + `
}
`
}

func TestListenerRulePriorityIsAllocatedOnceKeptAndTakenBackFromItsRule(t *testing.T) {
	listener := &mockListener{rules: []mockRule{{priority: "1000", tags: map[string]string{"mock-owner": "mock-z"}}}}
	withMockListener(t, listener)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protocolProviders,
		Steps: []resource.TestStep{
			{
				Config: allocationConfig(1002, `{ mock-role = "mock-api" }`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("awsextras_lb_listener_rule_priority.mock", tfjsonpath.New("priority"),
						knownvalue.Int64Exact(1001)),
					statecheck.ExpectKnownValue("awsextras_lb_listener_rule_priority.mock", tfjsonpath.New("priority_tag"),
						knownvalue.MapExact(map[string]knownvalue.Check{priorityTagKey: knownvalue.StringExact("1001")})),
					statecheck.ExpectKnownValue("awsextras_lb_listener_rule_priority.mock", tfjsonpath.New("tags_all"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mock-owner": knownvalue.StringExact("mock-a"),
							"mock-role":  knownvalue.StringExact("mock-api"),
						})),
				},
			},
			{
				PreConfig: func() {
					listener.mutex.Lock()
					defer listener.mutex.Unlock()
					listener.rules = append(listener.rules,
						mockRule{priority: "1001", tags: map[string]string{
							priorityTagKey: "1001", "mock-owner": "mock-a", "mock-role": "mock-api",
						}},
						mockRule{priority: "1002", tags: map[string]string{}},
					)
				},
				Config: allocationConfig(1002, `{ mock-role = "mock-api" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: allocationConfig(1003, `{ mock-role = "mock-api" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("awsextras_lb_listener_rule_priority.mock", plancheck.ResourceActionReplace),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("awsextras_lb_listener_rule_priority.mock", tfjsonpath.New("priority"),
						knownvalue.Int64Exact(1001)),
				},
			},
			{
				Config:      allocationConfig(1002, `{ mock-role = "mock-other" }`),
				ExpectError: regexp.MustCompile(`every priority from 1000 to 1002 is held`),
			},
		},
	})
}

func TestListenerRulePriorityRefusesARangeThatEndsBelowItsStart(t *testing.T) {
	withMockListener(t, &mockListener{})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protocolProviders,
		Steps: []resource.TestStep{{
			Config: `
resource "awsextras_lb_listener_rule_priority" "mock" {
  listener_arn = "` + mockListenerARN + `"
  min_priority = 2000
  max_priority = 1000
}
`,
			ExpectError: regexp.MustCompile(`min_priority is above max_priority`),
		}},
	})
}
