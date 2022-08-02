(import 'configmap.libsonnet') +
(import 'config.libsonnet') +
{
  local k = import 'ksonnet-util/kausal.libsonnet',
  local container = k.core.v1.container,
  local containerPort = k.core.v1.containerPort,
  local volumeMount = k.core.v1.volumeMount,
  local pvc = k.core.v1.persistentVolumeClaim,
  local statefulset = k.apps.v1.statefulSet,
  local volume = k.core.v1.volume,
  local service = k.core.v1.service,
  local servicePort = service.mixin.spec.portsType,
  local deployment = k.apps.v1.deployment,

  local target_name = 'scalable-single-binary',
  local tempo_config_volume = 'tempo-conf',
  local tempo_query_config_volume = 'tempo-query-conf',
  local tempo_data_volume = 'tempo-data',

  namespace:
    k.core.v1.namespace.new($._config.namespace),

  tempo_pvc:
    pvc.new() +
    pvc.mixin.spec.resources
    .withRequests({ storage: $._config.tempo_ssb.pvc_size }) +
    pvc.mixin.spec
    .withAccessModes(['ReadWriteOnce']) +
    pvc.mixin.spec
    .withStorageClassName($._config.tempo_ssb.pvc_storage_class) +
    pvc.mixin.metadata
    .withLabels({ app: 'tempo' }) +
    pvc.mixin.metadata
    .withNamespace($._config.namespace) +
    pvc.mixin.metadata
    .withName(tempo_data_volume) +
    { kind: 'PersistentVolumeClaim', apiVersion: 'v1' },

  tempo_container::
    container.new('tempo', $._images.tempo) +
    container.withPorts([
      containerPort.new('prom-metrics', $._config.port),
    ]) +
    container.withArgs([
      '-config.file=/conf/tempo.yaml',
      '-mem-ballast-size-mbs=' + $._config.ballast_size_mbs,
    ]) +
    container.withVolumeMounts([
      volumeMount.new(tempo_config_volume, '/conf'),
      volumeMount.new(tempo_data_volume, '/var/tempo'),
    ]) +
    $.util.withResources($._config.tempo_ssb.resources),

  tempo_statefulset:
    statefulset.new('tempo',
                    $._config.tempo_ssb.replicas,
                    [
                      $.tempo_container,
                    ],
                    self.tempo_pvc,
                    { app: 'tempo' }) +
    statefulset.mixin.spec.withServiceName('tempo') +
    statefulset.mixin.spec.template.metadata.withAnnotations({
      config_hash: std.md5(std.toString($.tempo_configmap.data['tempo.yaml'])),
    }) +
    statefulset.mixin.metadata.withLabels({ app: $._config.tempo_ssb.headless_service_name, name: 'tempo' }) +
    statefulset.mixin.spec.selector.withMatchLabels({ name: 'tempo' }) +
    statefulset.mixin.spec.template.metadata.withLabels({ name: 'tempo', app: $._config.tempo_ssb.headless_service_name }) +
    statefulset.mixin.spec.template.spec.withVolumes([
      volume.fromConfigMap(tempo_query_config_volume, $.tempo_query_configmap.metadata.name),
      volume.fromConfigMap(tempo_config_volume, $.tempo_configmap.metadata.name),
    ]),

  tempo_service:
    k.util.serviceFor($.tempo_statefulset),


  tempo_headless_service:
    service.new(
      $._config.tempo_ssb.headless_service_name,
      { app: $._config.tempo_ssb.headless_service_name },
      []
    ) +
    service.mixin.spec.withClusterIP('None') +
    service.mixin.spec.withPublishNotReadyAddresses(true),

  util+:: {
    local k = import 'ksonnet-util/kausal.libsonnet',
    local container = k.core.v1.container,

    withResources(resources)::
      k.util.resourcesRequests(resources.requests.cpu, resources.requests.memory) +
      k.util.resourcesLimits(resources.limits.cpu, resources.limits.memory),

    readinessProbe::
      container.mixin.readinessProbe.httpGet.withPath('/ready') +
      container.mixin.readinessProbe.httpGet.withPort($._config.port) +
      container.mixin.readinessProbe.withInitialDelaySeconds(15) +
      container.mixin.readinessProbe.withTimeoutSeconds(1),
  },
}
