# Application Load Balancer
resource "aws_lb" "this" {
  name               = "${var.project}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [var.alb_security_group_id]
  subnets            = var.public_subnet_ids

  tags = { Name = "${var.project}-alb" }
}

# Target Groups

# Waiting Room Service — handles queue position assignment
resource "aws_lb_target_group" "waiting_room" {
  name        = "${var.project}-waiting-room-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip" # Required for Fargate awsvpc networking

  health_check {
    path                = "/health"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = { Name = "${var.project}-waiting-room-tg" }
}

# Flash Sale API — handles inventory decrement and order publishing
resource "aws_lb_target_group" "flash_sale_api" {
  name        = "${var.project}-api-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/health"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = { Name = "${var.project}-api-tg" }
}

# Listener + Routing Rules
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  # Default: route to flash sale API
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.flash_sale_api.arn
  }
}

# Route /queue/* to waiting room service
resource "aws_lb_listener_rule" "waiting_room" {
  listener_arn = aws_lb_listener.http.arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.waiting_room.arn
  }

  condition {
    path_pattern {
      values = ["/queue/*", "/queue"]
    }
  }
}

# Route /purchase/* to flash sale API
resource "aws_lb_listener_rule" "flash_sale_api" {
  listener_arn = aws_lb_listener.http.arn
  priority     = 20

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.flash_sale_api.arn
  }

  condition {
    path_pattern {
      values = ["/purchase/*", "/purchase", "/health", "/inventory"]
    }
  }
}
