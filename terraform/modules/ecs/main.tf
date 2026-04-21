resource "aws_ecs_cluster" "this" {
  name = "${var.project}-cluster"
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
  tags = { Name = "${var.project}-cluster" }
}

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

# ── Flash Sale API ─────────────────────────────────────────────────────────────
resource "aws_ecs_task_definition" "flash_sale_api" {
  family                   = "${var.project}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name      = "flash-sale-api"
    image     = var.flash_sale_api_image
    essential = true
    portMappings = [{ containerPort = 8080 }]
    environment = [
      { name = "REDIS_ENDPOINT",  value = var.redis_endpoint },
      { name = "REDIS_PORT",      value = "6379" },
      { name = "SQS_QUEUE_URL",   value = var.sqs_queue_url },
      { name = "INVENTORY_COUNT", value = tostring(var.inventory_count) },
      { name = "AWS_REGION",      value = var.aws_region },
      { name = "APP_PORT",        value = "8080" },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = var.flash_sale_api_log_group
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "flash_sale_api" {
  name            = "${var.project}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.flash_sale_api.arn
  desired_count   = var.flash_sale_api_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [var.ecs_security_group_id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = var.flash_sale_api_target_group_arn
    container_name   = "flash-sale-api"
    container_port   = 8080
  }

  lifecycle {
    ignore_changes = [desired_count]
  }
}

# ── Order Worker ───────────────────────────────────────────────────────────────
resource "aws_ecs_task_definition" "order_worker" {
  family                   = "${var.project}-order-worker"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name      = "order-worker"
    image     = var.order_worker_image
    essential = true
    environment = [
      { name = "SQS_QUEUE_URL",   value = var.sqs_queue_url },
      { name = "DYNAMODB_TABLE",  value = var.dynamodb_orders_table },
      { name = "INVENTORY_TABLE", value = var.dynamodb_inventory_table },
      { name = "NUM_WORKERS",     value = tostring(var.order_worker_goroutines) },
      { name = "AWS_REGION",      value = var.aws_region },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = var.order_worker_log_group
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "order_worker" {
  name            = "${var.project}-order-worker"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.order_worker.arn
  desired_count   = var.order_worker_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [var.ecs_security_group_id]
    assign_public_ip = true
  }

  lifecycle {
    ignore_changes = [desired_count]
  }
}

# ── Waiting Room ───────────────────────────────────────────────────────────────
resource "aws_ecs_task_definition" "waiting_room" {
  family                   = "${var.project}-waiting-room"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name      = "waiting-room"
    image     = var.waiting_room_image
    essential = true
    portMappings = [{ containerPort = 8080 }]
    environment = [
      { name = "REDIS_ENDPOINT",  value = var.redis_endpoint },
      { name = "REDIS_PORT",      value = "6379" },
      { name = "ADMISSION_RATE",  value = tostring(var.admission_rate) },
      { name = "QUEUE_STRATEGY",  value = var.queue_strategy },
      { name = "ITEM_ID",         value = "flash-sale-item" },
      { name = "AWS_REGION",      value = var.aws_region },
      { name = "APP_PORT",        value = "8080" },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = var.waiting_room_log_group
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "waiting_room" {
  name            = "${var.project}-waiting-room"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.waiting_room.arn
  desired_count   = var.waiting_room_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [var.ecs_security_group_id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = var.waiting_room_target_group_arn
    container_name   = "waiting-room"
    container_port   = 8080
  }

  lifecycle {
    ignore_changes = [desired_count]
  }
}

# ── Auto Scaling — Flash Sale API (CPU) ────────────────────────────────────────
resource "aws_appautoscaling_target" "flash_sale_api" {
  max_capacity       = 4
  min_capacity       = 1
  resource_id        = "service/${aws_ecs_cluster.this.name}/${aws_ecs_service.flash_sale_api.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "flash_sale_api_cpu" {
  name               = "${var.project}-api-cpu-scaling"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.flash_sale_api.resource_id
  scalable_dimension = aws_appautoscaling_target.flash_sale_api.scalable_dimension
  service_namespace  = aws_appautoscaling_target.flash_sale_api.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70.0
    scale_in_cooldown  = 60
    scale_out_cooldown = 30
  }
}

# ── Auto Scaling — Order Worker (SQS depth) ────────────────────────────────────
resource "aws_appautoscaling_target" "order_worker" {
  max_capacity       = 4
  min_capacity       = 1
  resource_id        = "service/${aws_ecs_cluster.this.name}/${aws_ecs_service.order_worker.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "order_worker_sqs" {
  name               = "${var.project}-worker-sqs-scaling"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.order_worker.resource_id
  scalable_dimension = aws_appautoscaling_target.order_worker.scalable_dimension
  service_namespace  = aws_appautoscaling_target.order_worker.service_namespace

  target_tracking_scaling_policy_configuration {
    customized_metric_specification {
      metric_name = "ApproximateNumberOfMessagesVisible"
      namespace   = "AWS/SQS"
      statistic   = "Average"
      dimensions {
        name  = "QueueName"
        value = var.sqs_queue_name
      }
    }
    target_value       = 100.0
    scale_in_cooldown  = 120
    scale_out_cooldown = 30
  }
}
