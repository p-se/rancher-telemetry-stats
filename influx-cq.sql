

# containers agg 1h, 4h, 12h 24h
CREATE CONTINUOUS QUERY "containers_1h" ON "telemetry" BEGIN SELECT last("container_total") AS "total", last("container_running") AS "running" INTO "containers_4h" FROM "telemetry" GROUP BY time(1h),uid END

CREATE CONTINUOUS QUERY "containers_4h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("running") AS "running" INTO "containers_4h" FROM "containers_1h" GROUP BY time(4h),uid END

CREATE CONTINUOUS QUERY "containers_12h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("running") AS "running" INTO "containers_12h" FROM "containers_4h" GROUP BY time(12h),uid END

CREATE CONTINUOUS QUERY "containers_24h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("running") AS "running" INTO "containers_24h" FROM "containers_12h" GROUP BY time(24h),uid END

# Installations agg 1h, 4h, 12h 24h
CREATE CONTINUOUS QUERY "installations_1h" ON "telemetry" BEGIN SELECT count(distinct("uid")) AS "total" INTO "installations_1h" FROM "telemetry" GROUP BY time(1h),uid END

CREATE CONTINUOUS QUERY "installations_4h" ON "telemetry" BEGIN SELECT last("total") AS "total" INTO "installations_4h" FROM "installations_1h" GROUP BY time(4h),uid END

CREATE CONTINUOUS QUERY "installations_12h" ON "telemetry" BEGIN SELECT last("total") AS "total" INTO "installations_12h" FROM "installations_4h" GROUP BY time(12h),uid END

CREATE CONTINUOUS QUERY "installations_24h" ON "telemetry" BEGIN SELECT last("total") AS "total" INTO "installations_24h" FROM "installations_12h" GROUP BY time(24h),uid END

# Countries agg 1h, 4h, 12h 24h
CREATE CONTINUOUS QUERY "by_country_1h" ON "telemetry" BEGIN SELECT count(distinct("uid")) AS "total" INTO "by_country_1h" FROM "telemetry" GROUP BY time(1h),country,country_isocode END

CREATE CONTINUOUS QUERY "by_country_4h" ON "telemetry" BEGIN SELECT last("total") AS "total" INTO "by_country_4h" FROM "by_country_1h" GROUP BY time(4h),country,country_isocode END

CREATE CONTINUOUS QUERY "by_country_12h" ON "telemetry" BEGIN SELECT last("total") AS "total" INTO "by_country_12h" FROM "by_country_4h" GROUP BY time(12h),country,country_isocode END

CREATE CONTINUOUS QUERY "by_country_24h" ON "telemetry" BEGIN SELECT last("total") AS "total" INTO "by_country_24h" FROM "by_country_12h" GROUP BY time(24h),country,country_isocode END

# orchestrators agg 1h, 4h, 12h 24h
CREATE CONTINUOUS QUERY "orchestrators_1h" ON "telemetry" BEGIN SELECT last("orch_kubernetes") AS "kubernetes", last("orch_mesos") AS "mesos", last("orch_swarm") AS "swarm", last("orch_windows") AS "windows", last("orch_cattle") AS "cattle" INTO "orchestrators_1h" FROM "telemetry" GROUP BY time(1h),uid END

CREATE CONTINUOUS QUERY "orchestrators_4h" ON "telemetry" BEGIN SELECT last("kubernetes") AS "kubernetes", last("mesos") AS "mesos", last("swarm") AS "swarm", last("windows") AS "windows", last("cattle") AS "cattle" INTO "orchestrators_4h" FROM "orchestrators_1h" GROUP BY time(4h),uid END

CREATE CONTINUOUS QUERY "orchestrators_12h" ON "telemetry" BEGIN SELECT last("kubernetes") AS "kubernetes", last("mesos") AS "mesos", last("swarm") AS "swarm", last("windows") AS "windows", last("cattle") AS "cattle" INTO "orchestrators_12h" FROM "orchestrators_4h" GROUP BY time(12h),uid END

CREATE CONTINUOUS QUERY "orchestrators_24h" ON "telemetry" BEGIN SELECT last("kubernetes") AS "kubernetes", last("mesos") AS "mesos", last("swarm") AS "swarm", last("windows") AS "windows", last("cattle") AS "cattle" INTO "orchestrators_24h" FROM "orchestrators_12h" GROUP BY time(24h),uid END


# services agg 1h, 4h, 12h 24h
CREATE CONTINUOUS QUERY "services_1h" ON "telemetry" BEGIN SELECT last("service_total") AS "total", last("service_active") AS "active" INTO "services_1h" FROM "telemetry" GROUP BY time(1h),uid END

CREATE CONTINUOUS QUERY "services_4h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("active") AS "active" INTO "services_4h" FROM "services_1h" GROUP BY time(4h),uid END

CREATE CONTINUOUS QUERY "services_12h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("active") AS "active" INTO "services_12h" FROM "services_4h" GROUP BY time(12h),uid END

CREATE CONTINUOUS QUERY "services_24h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("active") AS "active" INTO "services_24h" FROM "services_12h" GROUP BY time(24h),uid END

# stacks agg 1h, 4h, 12h 24h
CREATE CONTINUOUS QUERY "stacks_1h" ON "telemetry" BEGIN SELECT last("stack_total") AS "total", last("stack_active") AS "active" INTO "stacks_1h" FROM "telemetry" GROUP BY time(1h),uid END

CREATE CONTINUOUS QUERY "stacks_4h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("active") AS "active" INTO "stacks_4h" FROM "stacks_1h" GROUP BY time(4h),uid END

CREATE CONTINUOUS QUERY "stacks_12h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("active") AS "active" INTO "stacks_12h" FROM "stacks_4h" GROUP BY time(12h),uid END

CREATE CONTINUOUS QUERY "stacks_24h" ON "telemetry" BEGIN SELECT last("total") AS "total", last("active") AS "active" INTO "stacks_24h" FROM "stacks_12h" GROUP BY time(24h),uid END

# hosts agg 4h
CREATE CONTINUOUS QUERY "hosts_1h" ON "telemetry" BEGIN SELECT last("host_active") AS "host", last("host_cpu_cores_total") AS "cpu", last("host_mem_mb_total") AS "memory" INTO "hosts_1h" FROM "telemetry" GROUP BY time(1h),uid END

CREATE CONTINUOUS QUERY "hosts_4h" ON "telemetry" BEGIN SELECT last("host") AS "host", last("cpu") AS "cpu", last("memory") AS "memory" INTO "hosts_4h" FROM "hosts_1h" GROUP BY time(4h),uid END

CREATE CONTINUOUS QUERY "hosts_12h" ON "telemetry" BEGIN SELECT last("host") AS "host", last("cpu") AS "cpu", last("memory") AS "memory" INTO "hosts_12h" FROM "hosts_4h" GROUP BY time(12h),uid END

CREATE CONTINUOUS QUERY "hosts_24h" ON "telemetry" BEGIN SELECT last("host") AS "host", last("cpu") AS "cpu", last("memory") AS "memory" INTO "hosts_24h" FROM "hosts_12h" GROUP BY time(24h),uid END
