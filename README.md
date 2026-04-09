# Helm template diff
Compare helm template output between different branches for specific helm charts

# Usage
To get all diffed charts in the current repository, run:
`helm-template-diff --env test --dry-run client`

If outside of the chart git repository, path to a specific helm chart or git repository containing helm charts can be passed e.g.:
`helm-template-diff --path /path/to/git/repo --env test --dry-run client`
or
`helm-template-diff --path /path/to/git/repo/helm --env test --dry-run client`

The `--env` flag will by default use `values.yaml` and `env-values.yaml` as values in the helm template command. If you need to use other value files, use the `--values` flag.

`helm-template-diff --dry-run client --values values.yaml --values other-values.yaml`
