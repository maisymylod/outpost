{{/* Common naming and label helpers for the chart. */}}

{{- define "outpost.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "outpost.fullname" -}}
{{- printf "%s" .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "outpost.labels" -}}
app.kubernetes.io/name: {{ include "outpost.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "outpost.selectorLabels" -}}
app.kubernetes.io/name: {{ include "outpost.name" . }}
app.kubernetes.io/component: operator
{{- end -}}
