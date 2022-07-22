{
  _images+:: {
    tempo: 'grafana/tempo:latest',
    tempo_query: 'grafana/tempo-query:latest',
    tempo_vulture: 'grafana/tempo-vulture:latest',
  },

  _config+:: {
    gossip_member_label: 'tempo-gossip-member',
    port: 3200,
    distributor+: {
      receivers: $._config.distributor.receivers,
    },
    ballast_size_mbs: '1024',
    jaeger_ui: {
      base_path: '/',
    },
    vulture: {
      replicas: 0,
      tempoPushUrl: 'http://tempo',
      tempoQueryUrl: 'http://tempo:%s' % $._config.port,
      tempoOrgId: '',
      tempoRetentionDuration: '',
      tempoSearchBackoffDuration: '',
      tempoReadBackoffDuration: '',
      tempoWriteBackoffDuration: '',
    },

    tempo_ssb: {
      replicas: 3,
      pvc_size: error 'Must specify an ingester pvc size',
      pvc_storage_class: error 'Must specify an ingester pvc storage class',
      resources: {
        requests: {
          cpu: '500m',
          memory: '1Gi',
        },
        limits: {
          cpu: '1',
          memory: '2Gi',
        },
      },
    },
  },
}
