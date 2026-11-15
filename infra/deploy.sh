#!/usr/bin/env bash
# Runs as root on the EC2 host, started by the pociag-deploy SSM document after it has checked
# out the commit. Usage: deploy.sh <full-commit-sha>
# Never print .env or `compose config`: this output ends up in public GitHub Actions logs.
set -euo pipefail

AWS_REGION=eu-central-1
NEW_TAG=$1
REPO=/opt/pociag
TAG_FILE=/opt/pociag-deployed-tag

export HOME=/root
export PATH="$PATH:/snap/bin"
cd "$REPO/infra"

compose() { docker compose --profile all -f docker-compose.yml -f docker-compose.prod.yml "$@"; }

PREV_TAG=$(cat "$TAG_FILE" 2>/dev/null || true)
# True when the paths differ from the last successful deploy (or on the first deploy).
changed() { [ -z "$PREV_TAG" ] || ! git -C "$REPO" diff --quiet "$PREV_TAG" "$NEW_TAG" -- "$@"; }

(
  umask 077
  aws ssm get-parameters-by-path --region "$AWS_REGION" --path /pociag/prod --with-decryption \
    --query 'Parameters[].[Name,Value]' --output text \
    | awk '{n=$1; sub(".*/","",n); print n"="$2}' > .env.new
  grep -q '^POSTGRES_PASSWORD=' .env.new
  echo "IMAGE_TAG=$NEW_TAG" >> .env.new
  mv .env.new .env
)

REGISTRY=$(sed -n 's/^ECR_REGISTRY=//p' .env)
aws ecr get-login-password --region "$AWS_REGION" \
  | docker login --username AWS --password-stdin "$REGISTRY" > /dev/null

compose pull --quiet api frontend
compose up -d

# Airflow imports plugins at start, and DAG/plugin files arrive through the checkout.
if changed airflow/; then
  compose restart airflow-scheduler airflow-webserver
fi
# Single-file bind mounts keep pointing at the old file until the container is recreated.
if changed infra/Caddyfile infra/otel-collector-config.yml infra/prometheus.yml; then
  compose up -d --force-recreate --no-deps caddy otel-collector prometheus
fi

status=unknown
for _ in $(seq 1 60); do
  status=$(docker inspect -f '{{.State.Health.Status}}' pociag-api-1)
  if [ "$status" = healthy ]; then
    break
  fi
  sleep 5
done
if [ "$status" != healthy ]; then
  echo "api did not become healthy (status: $status)" >&2
  compose logs --tail=50 api >&2
  exit 1
fi

docker image prune -af --filter until=168h > /dev/null
echo "$NEW_TAG" > "$TAG_FILE"
echo "Deployed $NEW_TAG"
