package awsextrasprovider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// priorityTagKey is the tag that records an allocated priority on its rule.
const priorityTagKey = "awsextras:lb-listener-rule-priority"

// The priorities a listener rule may have.
const (
	lowestRulePriority  = 1
	highestRulePriority = 50000
)

// tagsPerDescribeTagsCall is how many resources one DescribeTags call reads.
const tagsPerDescribeTagsCall = 20

// listenerRules is what allocation reads of a listener.
type listenerRules interface {
	DescribeRules(context.Context, *elbv2.DescribeRulesInput, ...func(*elbv2.Options)) (*elbv2.DescribeRulesOutput, error)
	DescribeTags(context.Context, *elbv2.DescribeTagsInput, ...func(*elbv2.Options)) (*elbv2.DescribeTagsOutput, error)
}

// heldPriority is a priority a rule of the listener holds, with the rule's tags.
type heldPriority struct {
	priority int
	ruleARN  string
	tags     map[string]string
}

// listenerPriorities are the priorities the listener's rules hold, the default
// rule aside, each with its rule's tags.
func listenerPriorities(ctx context.Context, rules listenerRules, listenerARN string) ([]heldPriority, error) {
	var held []heldPriority
	var marker *string
	for {
		page, err := rules.DescribeRules(ctx, &elbv2.DescribeRulesInput{ListenerArn: aws.String(listenerARN), Marker: marker})
		if err != nil {
			return nil, fmt.Errorf("failed to list the rules of listener %s: %w", listenerARN, err)
		}
		for _, rule := range page.Rules {
			if aws.ToBool(rule.IsDefault) {
				continue
			}
			priority, err := strconv.Atoi(aws.ToString(rule.Priority))
			if err != nil {
				return nil, fmt.Errorf("rule %s of listener %s has the priority %q, not a number",
					aws.ToString(rule.RuleArn), listenerARN, aws.ToString(rule.Priority))
			}
			held = append(held, heldPriority{priority: priority, ruleARN: aws.ToString(rule.RuleArn), tags: map[string]string{}})
		}
		if aws.ToString(page.NextMarker) == "" {
			break
		}
		marker = page.NextMarker
	}
	for start := 0; start < len(held); start += tagsPerDescribeTagsCall {
		batch := held[start:min(start+tagsPerDescribeTagsCall, len(held))]
		arns := make([]string, len(batch))
		byARN := map[string]*heldPriority{}
		for index := range batch {
			arns[index] = batch[index].ruleARN
			byARN[batch[index].ruleARN] = &batch[index]
		}
		described, err := rules.DescribeTags(ctx, &elbv2.DescribeTagsInput{ResourceArns: arns})
		if err != nil {
			return nil, fmt.Errorf("failed to read the tags of the rules of listener %s: %w", listenerARN, err)
		}
		for _, description := range described.TagDescriptions {
			rule, found := byARN[aws.ToString(description.ResourceArn)]
			if !found {
				continue
			}
			for _, tag := range description.Tags {
				rule.tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}
		}
	}
	return held, nil
}

// allocatedPriority is the priority allocated from minimum to maximum: the one
// a rule owned by owner already records, else the lowest no rule holds.
func allocatedPriority(held []heldPriority, minimum, maximum int, owner map[string]string) (int, error) {
	if recorded, found := recordedPriority(held, minimum, maximum, owner); found {
		return recorded, nil
	}
	taken := map[int]bool{}
	for _, rule := range held {
		taken[rule.priority] = true
	}
	for priority := minimum; priority <= maximum; priority++ {
		if !taken[priority] {
			return priority, nil
		}
	}
	return 0, fmt.Errorf("every priority from %d to %d is held, by rules tagged: %s",
		minimum, maximum, holders(held, minimum, maximum))
}

// recordedPriority is the priority in range that a rule tagged with every owner
// tag records as allocated, which a lost state takes back.
func recordedPriority(held []heldPriority, minimum, maximum int, owner map[string]string) (int, bool) {
	if len(owner) == 0 {
		return 0, false
	}
	for _, rule := range held {
		if rule.priority < minimum || rule.priority > maximum || rule.tags[priorityTagKey] != strconv.Itoa(rule.priority) {
			continue
		}
		owned := true
		for key, value := range owner {
			if rule.tags[key] != value {
				owned = false
				break
			}
		}
		if owned {
			return rule.priority, true
		}
	}
	return 0, false
}

// holders says who holds the priorities in range: each set of rule tags, the
// allocation tag aside, with how many priorities it holds.
func holders(held []heldPriority, minimum, maximum int) string {
	counts := map[string]int{}
	for _, rule := range held {
		if rule.priority < minimum || rule.priority > maximum {
			continue
		}
		var pairs []string
		for key, value := range rule.tags {
			if key != priorityTagKey {
				pairs = append(pairs, key+"="+value)
			}
		}
		sort.Strings(pairs)
		described := strings.Join(pairs, " ")
		if described == "" {
			described = "(no tags)"
		}
		counts[described]++
	}
	described := make([]string, 0, len(counts))
	for tags, count := range counts {
		described = append(described, fmt.Sprintf("%s (%d)", tags, count))
	}
	sort.Strings(described)
	return strings.Join(described, ", ")
}
