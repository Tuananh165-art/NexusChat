{{- define "nexuschat.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "nexuschat.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- include "nexuschat.name" . -}}
{{- end -}}
{{- end -}}

{{- define "nexuschat.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "nexuschat.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nexuschat
{{- end -}}

{{- define "nexuschat.serviceLabels" -}}
{{ include "nexuschat.labels" .root }}
app.kubernetes.io/component: {{ .name }}
{{- end -}}

{{- define "nexuschat.image" -}}
{{- if .svc.image.fullname -}}
{{- .svc.image.fullname -}}
{{- else -}}
{{- $registry := trimSuffix "/" (default .root.Values.global.imageRegistry .svc.image.registry) -}}
{{- $repository := .svc.image.repository -}}
{{- $tag := default .root.Values.imageDefaults.tag .svc.image.tag -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "nexuschat.ingressName" -}}
{{- printf "%s-%s" (include "nexuschat.fullname" .root) .name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "nexuschat.traefikMiddlewareName" -}}
{{- printf "%s-%s-forward-auth" (include "nexuschat.fullname" .root) .name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "nexuschat.traefikMiddlewareRef" -}}
{{- printf "%s-%s@kubernetescrd" .root.Release.Namespace (include "nexuschat.traefikMiddlewareName" .) -}}
{{- end -}}
