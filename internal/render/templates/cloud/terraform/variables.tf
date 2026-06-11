variable "region" {
  type        = string
  description = "AWS region for the cluster."
  default     = "us-east-1"
}

variable "cluster_name" {
  type        = string
  description = "Name of the EKS cluster."
}

variable "kubernetes_version" {
  type        = string
  description = "EKS control plane version."
  default     = "1.30"
}

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the cluster VPC."
  default     = "10.0.0.0/16"
}

variable "gpu_instance_type" {
  type        = string
  description = "EC2 instance type for the GPU node group."
}

variable "gpu_node_desired" {
  type        = number
  description = "Desired GPU node count."
}

variable "gpu_node_min" {
  type        = number
  description = "Minimum GPU node count."
}

variable "gpu_node_max" {
  type        = number
  description = "Maximum GPU node count."
}

variable "workload_namespace" {
  type        = string
  description = "Namespace for the workload Helm release."
}

variable "release_name" {
  type        = string
  description = "Helm release name for the workload."
}

variable "workload_chart_path" {
  type        = string
  description = "Path to the outpost on-prem chart used to deploy the workload."
  default     = "../../on-prem/chart"
}
