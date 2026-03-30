# Subnet Group 
resource "aws_elasticache_subnet_group" "this" {
  name       = "${var.project}-redis-subnet-group"
  subnet_ids = var.private_subnet_ids

  tags = { Name = "${var.project}-redis-subnet-group" }
}

# Redis Cluster 
# Single-node cache.t3.micro for free tier / development
# TO UPGRADE TO CLUSTER MODE:
#   1. Change cluster_mode_enabled to true
#   2. Change node_type to "cache.r6g.large" or similar
#   3. Set num_cache_clusters to desired count
#   4. Add: automatic_failover_enabled = true
#   5. Add: multi_az_enabled = true
# Cluster mode gives you sharding + replicas for production workloads
resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${var.project}-redis"
  description          = "Redis for waiting room sorted sets and inventory DECR"

  # Free tier: cache.t3.micro single node
  # For production: cache.r6g.large with num_cache_clusters = 3
  node_type            = "cache.t3.micro"
  num_cache_clusters   = 1
  port                 = 6379

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [var.redis_security_group_id]

  # Disable at-rest encryption for dev (enable for production)
  at_rest_encryption_enabled = false
  transit_encryption_enabled = false

  # No automatic failover needed for single node
  automatic_failover_enabled = false

  apply_immediately = true

  tags = { Name = "${var.project}-redis" }
}
