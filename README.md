# Terraform Provider awsextras

AWS capabilities that the official AWS provider is missing, configured and tagged as the AWS provider is.

## Features

- `awsextras_lb_listener_rule_priority` allocates a free priority for a load balancer listener rule from a range, once, and keeps it.
- The allocated priority is recorded on its rule as a tag, so a lost state takes it back.
- A full range fails with the tags of the rules that hold it.

Each resource's arguments are on the [Terraform Registry](https://registry.terraform.io/providers/litebitus/awsextras/latest/docs); the design is in [docs/design.md](docs/design.md).

## Examples

| Resource | Example | Documentation |
|---|---|---|
| `awsextras_lb_listener_rule_priority` | [`examples/resources/awsextras_lb_listener_rule_priority/`](examples/resources/awsextras_lb_listener_rule_priority) | [docs/resources/lb_listener_rule_priority.md](docs/resources/lb_listener_rule_priority.md) |

## Usage

```hcl
terraform {
  required_providers {
    awsextras = {
      source = "litebitus/awsextras"
    }
  }
}

resource "awsextras_lb_listener_rule_priority" "mock" {
  listener_arn = aws_lb_listener.mock.arn
  min_priority = 1000
  max_priority = 2000
}
```

## Development

Go and Terraform build and test the repo; GoReleaser builds the snapshot archives; Docker runs the acceptance test's AWS API.

| Target | Does |
|---|---|
| `make build` | runs the tests, then builds the provider into `bin/` |
| `make test` | `go vet`, a `gofmt` check, the Go tests including plans through Terraform, and `tfplugindocs validate` |
| `make test-acceptance` | runs the acceptance test against the Moto server's AWS API in a container |
| `make docs` | generates the Registry pages in `docs/` from the resources and `examples/` |
| `make install` | builds the provider and prints the `dev_overrides` block that points local plans at it |
| `make snapshot` | runs the tests, then builds every platform's archive, unsigned, into `dist/` |

### Releases

The `Release` workflow, dispatched by hand, runs the tests, computes the next version, stamps `CHANGELOG.md`, tags it, and publishes the signed archives the Terraform Registry reads. It signs with the GPG key in the `GPG_PRIVATE_KEY` and `PASSPHRASE` secrets, whose public key is registered with the Registry.

The `Snapshot` workflow, dispatched by hand, publishes a signed pre-release that a configuration installs by pinning its exact version.
