package checks

// hardcodedSecrets flags credentials that a change adds as literal values.
var hardcodedSecrets = Check{
	Name:        "hardcoded-secrets",
	Description: "Flags keys, tokens, passwords and other credentials that the change adds as literal values",
	Prompt:      hardcodedSecretsPrompt,
}

const hardcodedSecretsPrompt = "flag secrets on added (`+`) lines only. report exactly these kinds of issues:\n" +
	"- provider keys and tokens: values with a known prefix or shape (e.g. `AKIA...`, `ghp_...`, `sk-...`, `xoxb-...`, a JWT `eyJ...`)\n" +
	"- private keys: a `-----BEGIN ... PRIVATE KEY-----` block\n" +
	"- credentials in URLs or connection strings (e.g. `postgres://user:pass@host`)\n" +
	"- credentials in auth headers (e.g. `Authorization: Basic ...`, `Bearer ...`)\n" +
	"- literal values for names like password, secret, token or api key, including a literal fallback for an environment lookup\n" +
	"leave unflagged: placeholders (e.g. `changeme`, `<your-key>`, `xxx`); values read from the environment or a secret store; names, paths or ids of secrets in a secret store (e.g. `secretName = \"prod/db\"`); numbers, lengths and limits (e.g. `passwordMinLength = 8`, `maxPromptTokens = 100_000`); public keys; hashes, checksums and UUIDs when their name or context does not mark them as credentials\n" +
	"list each finding with its file, its kind, and the added line as inline code with the secret value replaced by `***`, nothing else. keep no part of the value except a known public prefix (e.g. `AKIA`, `ghp_`). put each finding on its own line, followed by one blank line"
