eval "$GOSHIMMER_SEEDS"

ARGS=("$@")
ansible-playbook -u root -i deploy/ansible/hosts/"${1}" --extra-vars \
  "ANALYSISSENTRY_01_ENTRYNODE_SEED=$ANALYSISSENTRY_01_ENTRYNODE_SEED
BOOTSTRAP_01_SEED=$BOOTSTRAP_01_SEED
VANILLA_01_SEED=$VANILLA_01_SEED
DRNG_01_SEED=$DRNG_01_SEED
DRNG_02_SEED=$DRNG_02_SEED
DRNG_03_SEED=$DRNG_03_SEED
DRNG_04_SEED=$DRNG_04_SEED
DRNG_05_SEED=$DRNG_05_SEED
DRNG_XTEAM_01_SEED=$DRNG_XTEAM_01_SEED
FAUCET_01_SEED=$FAUCET_01_SEED
FAUCET_01_FAUCET_SEED=$FAUCET_01_FAUCET_SEED
drandsSecret=$DRANDS_SECRET
mongoDBUser=$MONGODB_USER
mongoDBPassword=$MONGODB_PASSWORD
assetRegistryUser=$ASSET_REGISTRY_USER
assetRegistryPassword=$ASSET_REGISTRY_PASSWORD
networkVersion=$NETWORK_VERSION
grafanaAdminPassword=$GRAFANA_ADMIN_PASSWORD
elkElasticUser=$ELK_ELASTIC_USER
elkElasticPassword=$ELK_ELASTIC_PASSWORD
goshimmerDockerImage=$GOSHIMMER_DOCKER_IMAGE
goshimmerDockerTag=$GOSHIMMER_DOCKER_TAG
snapshotterBucket=$SNAPSHOTTER_BUCKET
snapshotterAccessKey=$SNAPSHOTTER_ACCESS_KEY
snapshotterSecretKey=$SNAPSHOTTER_SECRET_KEY" \
  ${ARGS[@]:2} deploy/ansible/"${2:-deploy.yml}"
