#!/bin/sh
set -eu

if [ -n "${CLEARML_AGENT_TEMP_STDOUT_FILE_DIR:-}" ]; then
  mkdir -p "${CLEARML_AGENT_TEMP_STDOUT_FILE_DIR}"
  chmod 0777 "${CLEARML_AGENT_TEMP_STDOUT_FILE_DIR}"
fi

exec clearml-agent "$@"
