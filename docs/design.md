# awsextras design

The provider gives what the official AWS provider is missing, in the AWS provider's own terms: its configuration, `tags`, `default_tags` and `tags_all` work as the AWS provider's do.

## Configuration

| Setting | Holds |
|---|---|
| `region` | the AWS region; default, the AWS SDK's default configuration |
| `profile` | the shared configuration profile; default, the SDK's default |
| `assume_role` | `role_arn` and `session_name` of a role the provider assumes |
| `default_tags` | `tags` every resource takes, under its own `tags` |

The AWS SDK's own settings apply as they do to any SDK client, such as `AWS_ENDPOINT_URL`.

## `awsextras_lb_listener_rule_priority`

A listener rule needs a priority unique on its listener, and the AWS provider asks for that number by hand. This resource allocates it.

| Attribute | Holds |
|---|---|
| `listener_arn` | the listener the rule belongs to |
| `min_priority`, `max_priority` | the range to allocate from; default, the whole range AWS allows, `1` to `50000` |
| `tags` | the tags that identify whose rule the priority is for, over the provider's `default_tags`; `tags_all` is the two merged |
| `priority` | the allocated priority |
| `priority_tag` | the one tag that records the priority on its rule, `awsextras:lb-listener-rule-priority`, for the caller to merge into the rule's `tags` |

### Allocation

- On create, the resource lists the listener's rules (`DescribeRules`) and their tags (`DescribeTags`, 20 rules a call), and allocates the lowest priority in range that no rule holds.
- An allocated priority is kept. Every later plan reads it from the state and never lists the rules again, so a rule's priority never moves while other rules come and go.
- A change of the listener or the range allocates again.
- A rule that records an allocation in `priority_tag`, tagged with every tag of `tags_all`, holds the priority that a lost state takes back: on create, the resource takes that rule's priority before it looks for a free one.
- A range with no free priority fails the apply, naming the tags of the rules that hold it and how many priorities each holds.
- Two resources allocating on one listener at once can take the same number; the load balancer then refuses the second rule with `PriorityInUse`, and the next apply allocates again.
- Destroying the resource forgets the priority. The rule that holds it is the caller's.

## Tests

| Test | Runs against |
|---|---|
| allocation | the two ELB calls at the domain boundary, with paging and tag batching |
| the resource | Terraform, through the plugin testing framework, with the same boundary |
| acceptance | the ELB API of the [Moto](https://github.com/getmoto/moto) server in a container, with `make test-acceptance` |
