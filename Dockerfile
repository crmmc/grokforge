FROM python:3.13-slim AS curl-cffi

ARG CURL_CFFI_SPEC=curl_cffi

RUN pip install --no-cache-dir "${CURL_CFFI_SPEC}" && \
    mkdir -p /curl-libs && \
    python - <<'PY'
from pathlib import Path
import curl_cffi
import os
import shutil

package_dir = Path(curl_cffi.__file__).resolve().parent
search_roots = [package_dir, package_dir.parent / "curl_cffi.libs"]
output_dir = Path("/curl-libs")

copied = []
for root in search_roots:
    if not root.exists():
        continue
    for path in root.rglob("*.so*"):
        if not path.is_file():
            continue
        target = output_dir / path.name
        if target.exists():
            continue
        shutil.copy2(path, target)
        copied.append(target)

libcurl_candidates = sorted(
    path for path in copied
    if path.name.startswith("libcurl") and "impersonate" in path.name
)
if not libcurl_candidates:
    raise SystemExit("curl_cffi did not provide a libcurl-impersonate shared library")

preferred = next(
    (path for path in libcurl_candidates if "ff" not in path.name and "firefox" not in path.name),
    libcurl_candidates[0],
)
for link_name in ("libcurl.so.4", "libcurl.so"):
    link_path = output_dir / link_name
    try:
        link_path.unlink()
    except FileNotFoundError:
        pass
    os.symlink(preferred.name, link_path)
PY

FROM debian:bookworm-slim

ARG TARGETARCH

ENV LD_LIBRARY_PATH=/usr/local/lib

COPY --from=curl-cffi /curl-libs/ /usr/local/lib/

COPY dist/linux-${TARGETARCH}/grokforge-linux-${TARGETARCH} /usr/local/bin/grokforge
COPY config.defaults.toml /app/config.toml

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/* && \
    ldconfig && \
    chmod +x /usr/local/bin/grokforge && \
    useradd --system --uid 1000 --home-dir /app --shell /usr/sbin/nologin grokforge && \
    mkdir -p /app/data && \
    chown -R grokforge:grokforge /app

USER grokforge

WORKDIR /app
VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["grokforge"]
CMD ["-config", "/app/config.toml"]
