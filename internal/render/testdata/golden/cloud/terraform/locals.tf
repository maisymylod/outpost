data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, 2)

  tags = {
    "app.kubernetes.io/managed-by" = "outpost"
    "outpost/cluster"              = var.cluster_name
  }
}
