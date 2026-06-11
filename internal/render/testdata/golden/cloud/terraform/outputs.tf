output "cluster_name" {
  description = "EKS cluster name."
  value       = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  description = "EKS API server endpoint."
  value       = aws_eks_cluster.this.endpoint
}

output "gpu_node_group" {
  description = "Name of the GPU managed node group."
  value       = aws_eks_node_group.gpu.node_group_name
}
