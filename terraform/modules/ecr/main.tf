resource "aws_ecr_repository" "waiting_room" {
  name                 = "${var.project}-waiting-room"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  image_scanning_configuration { scan_on_push = false }
  tags = { Name = "${var.project}-waiting-room" }
}

resource "aws_ecr_repository" "flash_sale_api" {
  name                 = "${var.project}-api"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  image_scanning_configuration { scan_on_push = false }
  tags = { Name = "${var.project}-api" }
}

resource "aws_ecr_repository" "order_worker" {
  name                 = "${var.project}-order-worker"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  image_scanning_configuration { scan_on_push = false }
  tags = { Name = "${var.project}-order-worker" }
}
