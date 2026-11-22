package awsextrasprovider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure      = (*listenerRulePriorityResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*listenerRulePriorityResource)(nil)
	_ resource.ResourceWithValidateConfig = (*listenerRulePriorityResource)(nil)
)

// listenerRulePriorityResource allocates a free priority for a load balancer
// listener rule, once.
type listenerRulePriorityResource struct {
	data *providerData
}

func newListenerRulePriorityResource() resource.Resource {
	return &listenerRulePriorityResource{}
}

type listenerRulePriorityModel struct {
	ID          types.String `tfsdk:"id"`
	ListenerARN types.String `tfsdk:"listener_arn"`
	MinPriority types.Int64  `tfsdk:"min_priority"`
	MaxPriority types.Int64  `tfsdk:"max_priority"`
	Tags        types.Map    `tfsdk:"tags"`
	TagsAll     types.Map    `tfsdk:"tags_all"`
	Priority    types.Int64  `tfsdk:"priority"`
	PriorityTag types.Map    `tfsdk:"priority_tag"`
}

func (allocation *listenerRulePriorityResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_lb_listener_rule_priority"
}

func (allocation *listenerRulePriorityResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	priorityRange := []validator.Int64{int64validator.Between(lowestRulePriority, highestRulePriority)}
	response.Schema = schema.Schema{
		MarkdownDescription: "A free priority for a load balancer listener rule, allocated from a range once and kept.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The listener's ARN and the allocated priority.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"listener_arn": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ARN of the listener the rule belongs to. A change allocates again.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"min_priority": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(lowestRulePriority),
				MarkdownDescription: "The lowest priority to allocate. A change allocates again. Default: `1`.",
				Validators:          priorityRange,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"max_priority": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(highestRulePriority),
				MarkdownDescription: "The highest priority to allocate. A change allocates again. Default: `50000`.",
				Validators:          priorityRange,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"tags": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Tags that identify whose rule the priority is for, merged over the provider's `default_tags`.",
			},
			"tags_all": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "The provider's `default_tags` with `tags` over them.",
			},
			"priority": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The allocated priority.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"priority_tag": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "The one tag that records the allocated priority on its rule, to merge into the rule's tags.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (allocation *listenerRulePriorityResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}
	data, ok := request.ProviderData.(*providerData)
	if !ok {
		response.Diagnostics.AddError("unexpected provider data", fmt.Sprintf("got %T", request.ProviderData))
		return
	}
	allocation.data = data
}

func (allocation *listenerRulePriorityResource) ValidateConfig(ctx context.Context, request resource.ValidateConfigRequest, response *resource.ValidateConfigResponse) {
	var configured listenerRulePriorityModel
	response.Diagnostics.Append(request.Config.Get(ctx, &configured)...)
	if response.Diagnostics.HasError() {
		return
	}
	minimum, maximum := configured.MinPriority, configured.MaxPriority
	if minimum.IsUnknown() || maximum.IsUnknown() || minimum.IsNull() || maximum.IsNull() {
		return
	}
	if minimum.ValueInt64() > maximum.ValueInt64() {
		response.Diagnostics.AddAttributeError(path.Root("min_priority"), "min_priority is above max_priority",
			fmt.Sprintf("min_priority %d is above max_priority %d.", minimum.ValueInt64(), maximum.ValueInt64()))
	}
}

func (allocation *listenerRulePriorityResource) ModifyPlan(ctx context.Context, request resource.ModifyPlanRequest, response *resource.ModifyPlanResponse) {
	if request.Plan.Raw.IsNull() || allocation.data == nil {
		return
	}
	var planned listenerRulePriorityModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &planned)...)
	if response.Diagnostics.HasError() || planned.Tags.IsUnknown() {
		return
	}
	tagsAll, diagnostics := types.MapValueFrom(ctx, types.StringType, allocation.allTags(ctx, planned.Tags))
	response.Diagnostics.Append(diagnostics...)
	response.Diagnostics.Append(response.Plan.SetAttribute(ctx, path.Root("tags_all"), tagsAll)...)
}

func (allocation *listenerRulePriorityResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var planned listenerRulePriorityModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &planned)...)
	if response.Diagnostics.HasError() {
		return
	}
	listenerARN := planned.ListenerARN.ValueString()
	held, err := listenerPriorities(ctx, allocation.data.rules, listenerARN)
	if err != nil {
		response.Diagnostics.AddError("failed to read the listener's rules", err.Error())
		return
	}
	owner := allocation.allTags(ctx, planned.Tags)
	priority, err := allocatedPriority(held, int(planned.MinPriority.ValueInt64()), int(planned.MaxPriority.ValueInt64()), owner)
	if err != nil {
		response.Diagnostics.AddAttributeError(path.Root("listener_arn"), "no free priority on the listener", err.Error())
		return
	}
	planned.ID = types.StringValue(fmt.Sprintf("%s/%d", listenerARN, priority))
	planned.Priority = types.Int64Value(int64(priority))
	priorityTag, diagnostics := types.MapValueFrom(ctx, types.StringType, map[string]string{priorityTagKey: strconv.Itoa(priority)})
	response.Diagnostics.Append(diagnostics...)
	planned.PriorityTag = priorityTag
	response.Diagnostics.Append(response.State.Set(ctx, &planned)...)
}

// Read keeps the state as it is: an allocated priority is never allocated again.
func (allocation *listenerRulePriorityResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var stored listenerRulePriorityModel
	response.Diagnostics.Append(request.State.Get(ctx, &stored)...)
	response.Diagnostics.Append(response.State.Set(ctx, &stored)...)
}

func (allocation *listenerRulePriorityResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var planned, stored listenerRulePriorityModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &planned)...)
	response.Diagnostics.Append(request.State.Get(ctx, &stored)...)
	if response.Diagnostics.HasError() {
		return
	}
	planned.ID, planned.Priority, planned.PriorityTag = stored.ID, stored.Priority, stored.PriorityTag
	response.Diagnostics.Append(response.State.Set(ctx, &planned)...)
}

// Delete forgets the priority: the rule that holds it is the caller's.
func (allocation *listenerRulePriorityResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

// allTags is the provider's default tags with the resource's own over them.
func (allocation *listenerRulePriorityResource) allTags(ctx context.Context, own types.Map) map[string]string {
	merged := map[string]string{}
	for key, value := range allocation.data.defaultTags {
		merged[key] = value
	}
	if !own.IsNull() && !own.IsUnknown() {
		var tags map[string]string
		own.ElementsAs(ctx, &tags, false)
		for key, value := range tags {
			merged[key] = value
		}
	}
	return merged
}
