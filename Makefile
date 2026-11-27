.PHONY: build docs install snapshot test test-acceptance

MOTO_IMAGE = ghcr.io/litebitus/motoserver/moto:5.2.6
MOTO_CONTAINER = awsextras-acceptance-moto

build: test
	go build -o bin/terraform-provider-awsextras .

docs:
	go tool tfplugindocs generate --provider-name awsextras

install: build
	@printf '%s\n' \
		'Add to ~/.terraformrc for plans to use this build:' \
		'provider_installation {' \
		'  dev_overrides {' \
		'    "litebitus/awsextras" = "$(CURDIR)/bin"' \
		'  }' \
		'  direct {}' \
		'}'

snapshot: test
	goreleaser release --snapshot --clean --skip=sign

test:
	go vet -v ./...
	@unformatted="$$(gofmt -l .)"; [ -z "$$unformatted" ] || { echo "not gofmt-formatted:"; echo "$$unformatted"; exit 1; }
	go test ./...
	@if validated="$$(go tool tfplugindocs validate --provider-name awsextras 2>&1)"; then \
		echo "tfplugindocs validate: the Registry pages match the provider"; \
	else \
		printf '%s\n' "$$validated" | awk '/\[DEBUG\]/ { next } /^Error/ { failed = 1 } \
			{ if (failed) printf "\033[31m%s\033[0m\n", $$0; else print }'; \
		printf '\033[31m%s\033[0m\n' "tfplugindocs validate: failed, the Registry pages do not match the provider"; \
		exit 1; \
	fi

test-acceptance:
	@docker run -d --rm --name $(MOTO_CONTAINER) -p 127.0.0.1::5000 $(MOTO_IMAGE) >/dev/null
	@trap 'docker stop $(MOTO_CONTAINER) >/dev/null' EXIT; \
		port="$$(docker port $(MOTO_CONTAINER) 5000/tcp | head -1 | sed 's/.*://')"; \
		for attempt in $$(seq 60); do \
			curl -sf "http://127.0.0.1:$$port/moto-api/" >/dev/null && break; sleep 1; \
		done; \
		TF_ACC=1 AWS_ENDPOINT_URL="http://127.0.0.1:$$port" AWS_REGION=us-east-1 \
		AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
		go test ./... -run Acceptance -count=1
