# AWS inventory IAM (read-only)

The analyzer’s AWS inventory collector uses **Describe** APIs only. Attach a policy equivalent to:

- Embedded JSON: [internal/adapters/aws-inventory/iam-least-privilege.json](../internal/adapters/aws-inventory/iam-least-privilege.json)
- Capability manifest (API actions): [internal/adapters/aws-inventory/capabilities.yaml](../internal/adapters/aws-inventory/capabilities.yaml)

When using cross-account access, assume a role that trusts your caller and requires an external ID where appropriate. Credentials and session tokens are resolved at runtime and are **not** written to the workspace database or reports.
