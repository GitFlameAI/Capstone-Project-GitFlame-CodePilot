FROM debian:bookworm-slim AS binary-analyzers

ARG TARGETARCH
ARG GITLEAKS_VERSION=8.30.1
ARG OSV_SCANNER_VERSION=2.3.8

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir /out

RUN if [ "${TARGETARCH}" = "amd64" ]; then \
      gitleaks_arch=x64; gitleaks_sha=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb; \
      osv_sha=bc98e15319ed0d515e3f9235287ba53cdc5535d576d24fd573978ecfe9ab92dc; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
      gitleaks_arch=arm64; gitleaks_sha=e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080; \
      osv_sha=8158b18edd2d03b1a30d905ca91b032bc62262167be8f206c27114f08823e27c; \
    else exit 1; fi \
    && curl --fail --show-error --location \
      "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_${gitleaks_arch}.tar.gz" \
      --output /tmp/gitleaks.tar.gz \
    && echo "${gitleaks_sha}  /tmp/gitleaks.tar.gz" | sha256sum --check --strict \
    && tar -xzf /tmp/gitleaks.tar.gz -C /out gitleaks \
    && curl --fail --show-error --location \
      "https://github.com/google/osv-scanner/releases/download/v${OSV_SCANNER_VERSION}/osv-scanner_linux_${TARGETARCH}" \
      --output /out/osv-scanner \
    && echo "${osv_sha}  /out/osv-scanner" | sha256sum --check --strict \
    && chmod 0555 /out/gitleaks /out/osv-scanner \
    && rm -f /tmp/gitleaks.tar.gz

FROM debian:bookworm-slim AS pmd

ARG PMD_VERSION=7.24.0
ARG PMD_SHA256=110934b36d39c19094d1b77386931978093f238f2c2f1851748822b69c7367ac

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip \
    && curl --fail --show-error --location \
      "https://github.com/pmd/pmd/releases/download/pmd_releases%2F${PMD_VERSION}/pmd-dist-${PMD_VERSION}-bin.zip" \
      --output /tmp/pmd.zip \
    && echo "${PMD_SHA256}  /tmp/pmd.zip" | sha256sum --check --strict \
    && unzip -q /tmp/pmd.zip -d /opt \
    && mv "/opt/pmd-bin-${PMD_VERSION}" /opt/pmd \
    && rm -rf /var/lib/apt/lists/* /tmp/pmd.zip

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    AGENT_MODEL=laguna \
    OPENAI_BASE_URL=http://127.0.0.1:8000/v1 \
    MODEL_REQUEST_TIMEOUT_SECONDS=600 \
    ANALYZERS_ENABLED=true \
    OSV_CACHE_DIR=/var/cache/osv-scanner \
    PATH=/opt/pmd/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates openjdk-21-jre-headless passwd \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --system --gid 10001 codepilot \
    && useradd --system --uid 10001 --gid codepilot --home-dir /app codepilot \
    && mkdir -p /app /var/cache/osv-scanner \
    && chown -R codepilot:codepilot /app /var/cache/osv-scanner

COPY --from=binary-analyzers /out/gitleaks /usr/local/bin/gitleaks
COPY --from=binary-analyzers /out/osv-scanner /usr/local/bin/osv-scanner
COPY --from=pmd /opt/pmd /opt/pmd

WORKDIR /app
COPY --chown=codepilot:codepilot pyproject.toml ./
COPY --chown=codepilot:codepilot src ./src
COPY --chown=codepilot:codepilot space_start.sh ./

RUN sed -i 's/\r$//' space_start.sh \
    && chmod 0555 space_start.sh \
    && pip install --no-cache-dir .

USER 10001:10001

EXPOSE 7860
CMD ["./space_start.sh"]
