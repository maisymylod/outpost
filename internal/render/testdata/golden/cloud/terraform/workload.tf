# Deploy the workload onto the new cluster using the outpost on-prem chart and
# the spec-derived values rendered alongside this module.
resource "helm_release" "workload" {
  name             = var.release_name
  namespace        = var.workload_namespace
  create_namespace = true
  chart            = var.workload_chart_path

  values = [file("${path.module}/values.yaml")]

  depends_on = [aws_eks_node_group.gpu]
}
