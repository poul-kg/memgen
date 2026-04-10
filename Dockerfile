FROM golang:latest AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o memgen ./cmd/memgen

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git && \
    rm -rf /var/lib/apt/lists/*

# Install gh CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/memgen /usr/local/bin/memgen

# Create home directory for non-root user and a writable bin dir
RUN mkdir -p /home/memgen/.local/bin && chmod 777 /home/memgen/.local/bin

# Entrypoint script creates claude symlink in user-writable dir then runs memgen
RUN printf '#!/bin/sh\n\
CLAUDE_BIN=$(ls -t $HOME/.local/share/claude/versions/[0-9]* 2>/dev/null | head -1)\n\
if [ -n "$CLAUDE_BIN" ]; then\n\
  ln -sf "$CLAUDE_BIN" /home/memgen/.local/bin/claude\n\
fi\n\
export PATH="/home/memgen/.local/bin:$PATH"\n\
exec memgen "$@"\n' > /entrypoint.sh && chmod +x /entrypoint.sh

EXPOSE 3040

ENTRYPOINT ["/entrypoint.sh"]
