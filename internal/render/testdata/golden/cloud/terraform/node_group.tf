# GPU node group. AL2_x86_64_GPU ships the NVIDIA driver and container runtime;
# the taint keeps non-GPU pods off these (expensive) nodes, matched by the
# toleration the operator sets on workload pods.
resource "aws_eks_node_group" "gpu" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${var.cluster_name}-gpu"
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = aws_subnet.private[*].id

  ami_type       = "AL2_x86_64_GPU"
  instance_types = [var.gpu_instance_type]
  disk_size      = 200

  scaling_config {
    desired_size = var.gpu_node_desired
    min_size     = var.gpu_node_min
    max_size     = var.gpu_node_max
  }

  labels = {
    "nvidia.com/gpu" = "present"
  }

  taint {
    key    = "nvidia.com/gpu"
    value  = "present"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_worker,
    aws_iam_role_policy_attachment.node_cni,
    aws_iam_role_policy_attachment.node_ecr,
  ]
}
