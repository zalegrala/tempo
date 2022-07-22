{
  local k = import 'ksonnet-util/kausal.libsonnet',
  local configMap = k.core.v1.configMap,

  tempo_config:: {
    target: 'scalable-single-binary',
    server: {
      http_listen_port: $._config.port,
    },
    distributor: {
      receivers: $._config.distributor.receivers,
    },
    ingester: {
    },
    compactor: {
      compaction: {
        compacted_block_retention: '24h',
      },
      ring+: {
        kvstore+: {
          store: 'memberlist',
        },
      },
    },
    querier: {
      frontend_worker: {
        frontend_address: 'tempo.%s.svc.cluster.local:9095' % [$._config.namespace],
      },

    },
    storage: {
      trace: {
        blocklist_poll: '0',
        backend: $._config.backend,
        wal: {
          path: '/var/tempo/wal',
        },
        gcs: {
          bucket_name: $._config.bucket,
          chunk_buffer_size: 10485760,  // 1024 * 1024 * 10
        },
        s3: {
          bucket: $._config.bucket,
        },
        pool: {
          queue_depth: 2000,
        },
      },
    },
  },

  tempo_configmap:
    configMap.new('tempo') +
    configMap.withData({
      'tempo.yaml': k.util.manifestYaml($.tempo_config),
    }) +
    configMap.withDataMixin({
      'overrides.yaml': |||
        overrides:
      |||,
    }),

  tempo_query_configmap:
    configMap.new('tempo-query') +
    configMap.withData({
      'tempo-query.yaml': k.util.manifestYaml({
        backend: 'localhost:%d' % $._config.port,
      }),
    }),
}
