# Changelog

## Unreleased

- Resources
  - `awsextras_lb_listener_rule_priority`: a free priority for a load balancer listener rule, allocated from a range once and kept, so that a rule needs no hand-picked number.
- Provider
  - Configured as the AWS provider is, with `region`, `profile`, `assume_role` and `default_tags`.
- Development
  - `make build`, which tests first, `make test`, `make test-acceptance`, `make docs`, `make snapshot` and `make install`.
  - CI runs the tests, and the acceptance test against Moto, on every push and pull request.
  - Dispatched snapshot and release workflows publish the signed archives that the Terraform Registry reads.
