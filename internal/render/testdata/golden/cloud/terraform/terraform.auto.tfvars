# Rendered by outpost from the workload spec. Edit the spec, not this file.
region             = "us-east-1"
cluster_name       = "llm-inference"
workload_namespace = "inference"
release_name       = "llm-inference"
gpu_instance_type  = "p4de.24xlarge"
gpu_node_desired   = 2
gpu_node_min       = 2
gpu_node_max       = 4
