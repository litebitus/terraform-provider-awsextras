// Package awsextrasprovider is a Terraform provider for AWS capabilities that the
// official AWS provider is missing.
package awsextrasprovider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*awsextrasProvider)(nil)

type awsextrasProvider struct {
	version string
}

// New answers the provider constructor for one release version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &awsextrasProvider{version: version}
	}
}

type providerModel struct {
	Region      types.String      `tfsdk:"region"`
	Profile     types.String      `tfsdk:"profile"`
	AssumeRole  *assumeRoleModel  `tfsdk:"assume_role"`
	DefaultTags *defaultTagsModel `tfsdk:"default_tags"`
}

type assumeRoleModel struct {
	RoleARN     types.String `tfsdk:"role_arn"`
	SessionName types.String `tfsdk:"session_name"`
}

type defaultTagsModel struct {
	Tags map[string]string `tfsdk:"tags"`
}

// newListenerRules is the client that reads load balancers for an AWS
// configuration; a variable so a test states what the listener holds.
var newListenerRules = func(sdkConfig aws.Config) listenerRules { return elbv2.NewFromConfig(sdkConfig) }

// providerData is what the provider hands its resources: the client that reads
// load balancers, and the tags every resource takes as its own.
type providerData struct {
	rules       listenerRules
	defaultTags map[string]string
}

func (awsextras *awsextrasProvider) Metadata(_ context.Context, _ provider.MetadataRequest, response *provider.MetadataResponse) {
	response.TypeName = "awsextras"
	response.Version = awsextras.version
}

func (awsextras *awsextrasProvider) Schema(_ context.Context, _ provider.SchemaRequest, response *provider.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "AWS capabilities that the official AWS provider is missing.",
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The AWS region. Default: the region of the AWS SDK's default configuration.",
			},
			"profile": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The shared configuration profile. Default: the AWS SDK's default profile.",
			},
		},
		Blocks: map[string]schema.Block{
			"assume_role": schema.SingleNestedBlock{
				MarkdownDescription: "A role the provider assumes, as the AWS provider's `assume_role` block does.",
				Attributes: map[string]schema.Attribute{
					"role_arn": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "The ARN of the role to assume.",
					},
					"session_name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "The session name of the assumed role.",
					},
				},
			},
			"default_tags": schema.SingleNestedBlock{
				MarkdownDescription: "Tags every resource takes, as the AWS provider's `default_tags` block does.",
				Attributes: map[string]schema.Attribute{
					"tags": schema.MapAttribute{
						ElementType:         types.StringType,
						Optional:            true,
						MarkdownDescription: "The tags, merged under each resource's own `tags`.",
					},
				},
			},
		},
	}
}

func (awsextras *awsextrasProvider) Configure(ctx context.Context, request provider.ConfigureRequest, response *provider.ConfigureResponse) {
	var configured providerModel
	response.Diagnostics.Append(request.Config.Get(ctx, &configured)...)
	if response.Diagnostics.HasError() {
		return
	}
	var options []func(*awsconfig.LoadOptions) error
	if region := configured.Region.ValueString(); region != "" {
		options = append(options, awsconfig.WithRegion(region))
	}
	if profile := configured.Profile.ValueString(); profile != "" {
		options = append(options, awsconfig.WithSharedConfigProfile(profile))
	}
	sdkConfig, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		response.Diagnostics.AddError("failed to load the AWS configuration", err.Error())
		return
	}
	if configured.AssumeRole != nil && configured.AssumeRole.RoleARN.ValueString() != "" {
		assumed := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(sdkConfig), configured.AssumeRole.RoleARN.ValueString(),
			func(options *stscreds.AssumeRoleOptions) {
				if sessionName := configured.AssumeRole.SessionName.ValueString(); sessionName != "" {
					options.RoleSessionName = sessionName
				}
			})
		sdkConfig.Credentials = aws.NewCredentialsCache(assumed)
	}
	data := &providerData{rules: newListenerRules(sdkConfig), defaultTags: map[string]string{}}
	if configured.DefaultTags != nil {
		for key, value := range configured.DefaultTags.Tags {
			data.defaultTags[key] = value
		}
	}
	response.ResourceData = data
}

func (awsextras *awsextrasProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newListenerRulePriorityResource}
}

func (awsextras *awsextrasProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}
