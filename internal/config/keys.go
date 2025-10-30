package config

// Etcd server hostname
const ETCD_ADDRESS = "etcd.address"

// exposed port for serverledge APIs
const API_PORT = "api.port"
const API_IP = "api.ip"

// Forces runtime container images to be pulled the first time they are used,
// even if they are locally available (true/false).
const FACTORY_REFRESH_IMAGES = "factory.images.refresh"

// Amount of memory available for the container pool (in MB)
const POOL_MEMORY_MB = "container.pool.memory"

// CPUs available for the container pool (1.0 = 1 core)
const POOL_CPUS = "container.pool.cpus"

// periodically janitor wakes up and deletes expired containers
const POOL_CLEANUP_PERIOD = "janitor.interval"

// container expiration time
const CONTAINER_EXPIRATION_TIME = "container.expiration"

// cache capacity
const CACHE_SIZE = "cache.size"

// cache janitor interval (Seconds) : deletes expired items
const CACHE_CLEANUP = "cache.cleanup"

// default expiration time assigned to a cache item (Seconds)
const CACHE_ITEM_EXPIRATION = "cache.expiration"

// default policy is to persist cache (boolean). Use false in localonly deployments
const CACHE_PERSISTENCE = "cache.persistence"

// true if the current server is a remote cloud server
const IS_IN_CLOUD = "cloud"

// the area wich the server belongs to
const REGISTRY_AREA = "registry.area"

// the area that acts as "remote cloud" for this node
const REGISTRY_REMOTE_AREA = "registry.remote.area"

// short period: retrieve information about nearby edge-servers
const REG_NEARBY_INTERVAL = "registry.nearby.interval"

// long period for general monitoring inside the area
const REG_MONITORING_INTERVAL = "registry.monitoring.interval"

// port for udp status listener
const LISTEN_UDP_PORT = "registry.udp.port"

// enable metrics system
const METRICS_ENABLED = "metrics.enabled"

// Port used by Prometheus server
const METRICS_PROMETHEUS_PORT = "metrics.prometheus.port"

// Prometheus IP address / hostname
const METRICS_PROMETHEUS_HOST = "metrics.prometheus.host"

// Interval (in seconds) for metrics retriever
const METRICS_RETRIEVER_INTERVAL = "metrics.retriever.interval"

// Scheduling policy to use
// Possible values: "qosaware", "default", "cloudonly"
const SCHEDULING_POLICY = "scheduler.policy"

// Capacity of the queue (possibly) used by the scheduler
const SCHEDULER_QUEUE_CAPACITY = "scheduler.queue.capacity"

// Enables tracing
const TRACING_ENABLED = "tracing.enabled"

// Custom output file for traces
const TRACING_OUTFILE = "tracing.outfile"

// Workflow offloading policy to use
// Possible values: "disable", "ilp"
const WORKFLOW_OFFLOADING_POLICY = "workflow.offloading.policy"

// Port used by ILP Offloading Policy for solving the ILP formulation
const WORKFLOW_OFFLOADING_POLICY_OPTIMIZER_PORT = "workflow.offloading.policy.optimizer.port"

// IP address / hostname used by ILP Offloading Policy for solving the ILP formulation
const WORKFLOW_OFFLOADING_POLICY_OPTIMIZER_HOST = "workflow.offloading.policy.optimizer.host"

// Number of times a scheduling plan can be reused before being re-computed
const WORKFLOW_OFFLOADING_POLICY_ILP_PLACEMENT_TTL = "workflow.offloading.policy.ilp.placement.ttl"

// Monetary computation cost per region (Map: string -> float)
const WORKFLOW_OFFLOADING_POLICY_REGION_COST = "workflow.offloading.policy.region.cost"

// Weight of objective terms in the ILP offloading policy
const WORKFLOW_OFFLOADING_POLICY_ILP_OBJ_WEIGHT_VIOLATIONS = "workflow.offloading.policy.ilp.obj.violations"
const WORKFLOW_OFFLOADING_POLICY_ILP_OBJ_WEIGHT_DATA_TRANSFERS = "workflow.offloading.policy.ilp.obj.data"
const WORKFLOW_OFFLOADING_POLICY_ILP_OBJ_WEIGHT_COST = "workflow.offloading.policy.ilp.obj.cost"

// Estimated bandwidth between the current node and the data store
const WORKFLOW_OFFLOADING_POLICY_NODE_TO_DATA_STORE_BANDWIDTH = "workflow.offloading.policy.node2datastore.bandwidth"

// Estimated bandwidth between cloud nodes and the data store
const WORKFLOW_OFFLOADING_POLICY_CLOUD_TO_DATA_STORE_BANDWIDTH = "workflow.offloading.policy.cloud2datastore.bandwidth"

// Utilization threshold for the threshold-based offloading policy
const WORKFLOW_THRESHOLD_BASED_POLICY_THRESHOLD = "workflow.offloading.policy.threshold"

// Max number of tasks offloaded at once in the threshold-based offloading policy
const WORKFLOW_THRESHOLD_BASED_POLICY_MAX_OFFLOADED = "workflow.offloading.policy.threshold.offloaded.max"

const FUNCTION_OFFLOADING_POLICY_ILP_OBJ_WEIGHT_VIOLATIONS = "function.offloading.policy.ilp.obj.violations"
const FUNCTION_OFFLOADING_POLICY_ILP_OBJ_WEIGHT_COST = "function.offloading.policy.ilp.obj.cost"

// IP address / hostname used by ILP Offloading Policy for solving the ILP formulation
const FUNCTION_OFFLOADING_POLICY_OPTIMIZER_HOST = "function.offloading.policy.optimizer.host"

// Port used by ILP Offloading Policy for solving the ILP formulation
const FUNCTION_OFFLOADING_POLICY_OPTIMIZER_PORT = "function.offloading.policy.optimizer.port"

// Costo locale
const FUNCTION_OFFLOADING_POLICY_REGION_COST = "function.offloading.policy.region.cost"

// Cost Gb/Sec
const FUNCTION_OFFLOADING_COMPUTE_REGION_COST_GB_SEC = "function.offloading.compute.region.cost.gb.sec"

// Cost per request
const FUNCTION_OFFLOADING_COMPUTE_REGION_REQUEST_COST = "function.offloading.compute.region.request.cost"

const FUNCTION_OFFLOADING_POLICY_NODE_TO_DATA_STORE_BANDWIDTH = "function.offloading.policy.node2datastore.bandwidth"

const FUNCTION_OFFLOADING_POLICY_LAMBDA_TO_DATA_STORE_BANDWIDTH = "function.offloading.policy.lambda.to.data.store.bandwidth"

const FUNCTION_OFFLOADING_POLICY_UPDATE_INTERVAL = "function.offloading.policy.update.interval"

const EXTERNAL_PROVIDER_ENABLED = "external.provider.enabled"

const FUNCTION_OFFLOADING_BUDGET = "function.offloading.budget"

const FUNCTION_OFFLOADING_QOS_CLASSES_PATH = "function.offloading.qos.classes.path"

const POLICY_ARRIVAL_RATE_ALPHA = "policy.arrival.rate.alpha"

const POLICY_FUNCTION_NAME = "policy.function.name"

const FUNCTION_OFFLOADING_POLICY_LAMBDA_PING_INTERVAL = "function.offloading.policy.lambda.ping.interval"

// power consuming features
const PROCESSING_POWER_CONSUMPTION = "consumption.processing.power"
const TX_ENERGY_CONSUMPTION = "consumption.energy.tx"
const RX_ENERGY_CONSUMPTION = "consumption.energy.rx"

const REGIONS_FILE_PATH = "regions.file.path"

const OPTIMIZER_HOST = "optimizer.host"
const OPTIMIZER_PORT = "optimizer.port"
const ALPHA = "policy.alpha"
const BETA = "policy.beta"
const BUDGET = "budget"
const CO2_QOS_POLICY_GENERAL_CONFIG_PATH = "policy.general.config.path"
const POLICY_UPDATE_INTERVAL = "policy.update.interval"
