#!/usr/bin/env bash
# Runs in GitHub Actions: starts the deploy on the host through SSM, waits, then smoke-tests.
set -euo pipefail
: "${EC2_INSTANCE_ID:?}" "${PROD_URL:?}" "${IMAGE_TAG:?}"

command_id=$(aws ssm send-command --instance-ids "$EC2_INSTANCE_ID" --document-name pociag-deploy \
  --parameters "imageTag=$IMAGE_TAG" --timeout-seconds 300 \
  --query Command.CommandId --output text)
echo "SSM command: $command_id"

invocation() {
  aws ssm get-command-invocation --command-id "$command_id" \
    --instance-id "$EC2_INSTANCE_ID" --query "$1" --output text
}

status=Pending
for _ in $(seq 1 240); do
  sleep 5
  status=$(invocation Status 2>/dev/null || echo Pending)
  case "$status" in
    Success | Failed | Cancelled | TimedOut) break ;;
  esac
done

invocation StandardOutputContent
invocation StandardErrorContent >&2
if [ "$status" != Success ]; then
  echo "Deploy finished with status: $status" >&2
  exit 1
fi

for _ in $(seq 1 30); do
  if curl -fsS -m 10 -o /dev/null "$PROD_URL/bff/readyz" && curl -fsS -m 20 -o /dev/null "$PROD_URL/"; then
    echo "Smoke test passed"
    exit 0
  fi
  sleep 10
done
echo "Smoke test failed for $PROD_URL" >&2
exit 1
