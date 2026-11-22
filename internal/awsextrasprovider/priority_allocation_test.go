package awsextrasprovider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

const mockListenerARN = "arn:aws:elasticloadbalancing:mock-region:000000000000:listener/app/mock-lb/1/1"

// mockRule is a rule of the mock listener: its priority and its tags.
type mockRule struct {
	priority string
	tags     map[string]string
}

// mockListener answers DescribeRules a page at a time and DescribeTags for at
// most tagsPerDescribeTagsCall rules, as the service does.
type mockListener struct {
	mutex       sync.Mutex
	rules       []mockRule
	pageSize    int
	tagRequests int
}

func (listener *mockListener) arnOf(index int) string {
	return fmt.Sprintf("%s/mock-rule-%d", strings.Replace(mockListenerARN, ":listener/", ":listener-rule/", 1), index)
}

func (listener *mockListener) DescribeRules(_ context.Context, input *elbv2.DescribeRulesInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeRulesOutput, error) {
	listener.mutex.Lock()
	defer listener.mutex.Unlock()
	all := []elbv2types.Rule{{RuleArn: aws.String(listener.arnOf(-1)), Priority: aws.String("default"), IsDefault: aws.Bool(true)}}
	for index, rule := range listener.rules {
		all = append(all, elbv2types.Rule{RuleArn: aws.String(listener.arnOf(index)), Priority: aws.String(rule.priority)})
	}
	start := 0
	if input.Marker != nil {
		fmt.Sscanf(*input.Marker, "%d", &start)
	}
	pageSize := listener.pageSize
	if pageSize == 0 {
		pageSize = len(all)
	}
	end := min(start+pageSize, len(all))
	output := &elbv2.DescribeRulesOutput{Rules: all[start:end]}
	if end < len(all) {
		output.NextMarker = aws.String(fmt.Sprint(end))
	}
	return output, nil
}

func (listener *mockListener) DescribeTags(_ context.Context, input *elbv2.DescribeTagsInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeTagsOutput, error) {
	listener.mutex.Lock()
	defer listener.mutex.Unlock()
	listener.tagRequests++
	if len(input.ResourceArns) > tagsPerDescribeTagsCall {
		return nil, fmt.Errorf("mock: %d resources in one DescribeTags call", len(input.ResourceArns))
	}
	output := &elbv2.DescribeTagsOutput{}
	for _, arn := range input.ResourceArns {
		for index, rule := range listener.rules {
			if listener.arnOf(index) != arn {
				continue
			}
			description := elbv2types.TagDescription{ResourceArn: aws.String(arn)}
			for key, value := range rule.tags {
				description.Tags = append(description.Tags, elbv2types.Tag{Key: aws.String(key), Value: aws.String(value)})
			}
			output.TagDescriptions = append(output.TagDescriptions, description)
		}
	}
	return output, nil
}

func TestListenerPrioritiesReadsEveryPageAndEveryRulesTagsButTheDefaultRule(t *testing.T) {
	listener := &mockListener{pageSize: 3}
	for index := 0; index < 25; index++ {
		listener.rules = append(listener.rules, mockRule{
			priority: fmt.Sprint(1000 + index),
			tags:     map[string]string{"mock-owner": fmt.Sprint("mock-", index)},
		})
	}
	held, err := listenerPriorities(context.Background(), listener, mockListenerARN)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 25 || held[0].priority != 1000 || held[24].priority != 1024 {
		t.Fatalf("held %d priorities from %v to %v", len(held), held[0].priority, held[len(held)-1].priority)
	}
	if held[24].tags["mock-owner"] != "mock-24" || listener.tagRequests != 2 {
		t.Fatalf("tags of the last rule %v, in %d DescribeTags calls", held[24].tags, listener.tagRequests)
	}
}

func TestAllocatedPriorityIsTheLowestFreeInRange(t *testing.T) {
	held := []heldPriority{{priority: 999}, {priority: 1000}, {priority: 1002}}
	if priority, err := allocatedPriority(held, 1000, 2000, nil); err != nil || priority != 1001 {
		t.Fatalf("priority %d err %v, want 1001", priority, err)
	}
}

func TestAllocatedPriorityTakesBackThePriorityItsOwnersRuleRecords(t *testing.T) {
	owner := map[string]string{"mock-owner": "mock-a"}
	held := []heldPriority{
		{priority: 1000, tags: map[string]string{priorityTagKey: "1000", "mock-owner": "mock-b"}},
		{priority: 1005, tags: map[string]string{priorityTagKey: "1005", "mock-owner": "mock-a"}},
		{priority: 1003, tags: map[string]string{"mock-owner": "mock-a"}},
	}
	if priority, err := allocatedPriority(held, 1000, 2000, owner); err != nil || priority != 1005 {
		t.Fatalf("priority %d err %v, want the recorded 1005", priority, err)
	}
	if priority, _ := allocatedPriority(held, 1000, 2000, nil); priority != 1001 {
		t.Fatalf("with no owner tags, priority %d, want the lowest free 1001", priority)
	}
}

func TestAllocatedPriorityOfAFullRangeNamesItsHolders(t *testing.T) {
	held := []heldPriority{
		{priority: 1000, tags: map[string]string{priorityTagKey: "1000", "mock-owner": "mock-a"}},
		{priority: 1001, tags: map[string]string{"mock-owner": "mock-b"}},
		{priority: 1002, tags: map[string]string{}},
	}
	_, err := allocatedPriority(held, 1000, 1002, nil)
	want := "every priority from 1000 to 1002 is held, by rules tagged: (no tags) (1), mock-owner=mock-a (1), mock-owner=mock-b (1)"
	if err == nil || err.Error() != want {
		t.Fatalf("err %v, want %q", err, want)
	}
}
