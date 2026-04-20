resource "aws_elasticache_subnet_group" "this" {
  name       = "${var.project}-redis-subnet-group"
  subnet_ids = var.subnet_ids
  tags       = { Name = "${var.project}-redis-subnet-group" }
}

# Single-node cache.t3.micro — free tier eligible
# TO UPGRADE TO CLUSTER MODE:
#   1. Set num_cache_clusters = 3
#   2. Change node_type to "cache.r6g.large"
#   3. Set automatic_failover_enabled = true
#   4. Set multi_az_enabled = true
resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${var.project}-redis"
  description          = "Redis for waiting room sorted sets and inventory DECR"

  node_type          = "cache.t3.micro"
  num_cache_clusters = 1
  port               = 6379

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [var.redis_security_group_id]

  at_rest_encryption_enabled = false
  transit_encryption_enabled = false
  automatic_failover_enabled = false
  apply_immediately          = true

  tags = { Name = "${var.project}-redis" }
}
