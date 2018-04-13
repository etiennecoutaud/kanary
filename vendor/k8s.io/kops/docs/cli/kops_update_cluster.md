
<!--- This file is automatically generated by make gen-cli-docs; changes should be made in the go CLI command code (under cmd/kops) -->

## kops update cluster

Update a cluster.

### Synopsis


Create or update cloud or cluster resources to match current cluster state.  If the cluster or cloud resources already exist this command may modify those resources. 

If nodes need updating such as during a Kubernetes upgrade, a rolling-update may be required as well.

```
kops update cluster
```

### Examples

```
  # After cluster has been edited or upgraded, configure it with:
  kops update cluster k8s-cluster.example.com --yes --state=s3://kops-state-1234 --yes
```

### Options

```
      --create-kube-config                Will control automatically creating the kube config file on your local filesystem (default true)
      --lifecycle-overrides stringSlice   comma separated list of phase overrides, example: SecurityGroups=Ignore,InternetGateway=ExistsAndWarnIfChanges
      --model string                      Models to apply (separate multiple models with commas) (default "config,proto,cloudup")
      --out string                        Path to write any local output
      --phase string                      Subset of tasks to run: assets, cluster, network, security
      --ssh-public-key string             SSH public key to use (deprecated: use kops create secret instead)
      --target string                     Target - direct, terraform, cloudformation (default "direct")
  -y, --yes                               Create cloud resources, without --yes update is in dry run mode
```

### Options inherited from parent commands

```
      --alsologtostderr                  log to standard error as well as files
      --config string                    config file (default is $HOME/.kops.yaml)
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files (default false)
      --name string                      Name of cluster. Overrides KOPS_CLUSTER_NAME environment variable
      --state string                     Location of state storage. Overrides KOPS_STATE_STORE environment variable
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO
* [kops update](kops_update.md)	 - Update a cluster.
