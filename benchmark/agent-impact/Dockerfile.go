ARG GO_BASE=golang:1.26-bookworm
FROM ${GO_BASE}

RUN apt-get update \
  && apt-get install -y --no-install-recommends git ca-certificates python3 make g++ bash jq ripgrep time \
  && rm -rf /var/lib/apt/lists/*

COPY --from=node:24-bookworm /usr/local /usr/local

ENV PATH="/usr/local/go/bin:${PATH}"

RUN ln -sf /usr/local/go/bin/go /usr/local/bin/go \
  && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt \
  && npm install -g @openai/codex@0.128.0

WORKDIR /workspace
