module "network" {
  source  = "./modules/network"
  project = var.project
}

module "ecr" {
  source  = "./modules/ecr"
  project = var.project
}

module "logging" {
  source  = "./modules/logging"
  project = var.project
}

module "redis" {
  source                  = "./modules/redis"
  project                 = var.project
  private_subnet_ids      = module.network.private_subnet_ids
  redis_security_group_id = module.network.redis_security_group_id
}

module "sqs" {
  source            = "./modules/sqs"
  project           = var.project
  ecs_task_role_arn = "arn:aws:iam::125878857213:role/LabRole"
}

module "dynamodb" {
  source  = "./modules/dynamodb"
  project = var.project
}

module "alb" {
  source                = "./modules/alb"
  project               = var.project
  vpc_id                = module.network.vpc_id
  public_subnet_ids     = module.network.public_subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
}

module "ecs" {
  source = "./modules/ecs"

  project    = var.project
  aws_region = var.aws_region

  # Networking
  private_subnet_ids    = module.network.private_subnet_ids
  ecs_security_group_id = module.network.ecs_security_group_id

  # Container images
  waiting_room_image   = "${module.ecr.waiting_room_repo_url}:latest"
  flash_sale_api_image = "${module.ecr.flash_sale_api_repo_url}:latest"
  order_worker_image   = "${module.ecr.order_worker_repo_url}:latest"

  # ALB wiring
  waiting_room_target_group_arn   = module.alb.waiting_room_target_group_arn
  flash_sale_api_target_group_arn = module.alb.flash_sale_api_target_group_arn
  alb_listener_arn                = module.alb.alb_arn

  # Experiment variables — change these per test run
  admission_rate          = var.admission_rate
  inventory_count         = var.inventory_count
  order_worker_goroutines = var.order_worker_goroutines

  # Downstream resources
  redis_endpoint           = module.redis.redis_endpoint
  sqs_queue_url            = module.sqs.orders_queue_url
  sqs_queue_name           = "${var.project}-orders"
  dynamodb_orders_table    = module.dynamodb.orders_table_name
  dynamodb_inventory_table = module.dynamodb.inventory_table_name

  # Logging
  waiting_room_log_group   = module.logging.waiting_room_log_group
  flash_sale_api_log_group = module.logging.flash_sale_api_log_group
  order_worker_log_group   = module.logging.order_worker_log_group
}
