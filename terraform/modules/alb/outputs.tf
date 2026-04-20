output "alb_dns_name" {
  description = "ALB DNS — use as Locust target: http://<value>"
  value       = aws_lb.this.dns_name
}

output "alb_arn" {
  value = aws_lb.this.arn
}

output "flash_sale_api_target_group_arn" {
  value = aws_lb_target_group.flash_sale_api.arn
}

output "waiting_room_target_group_arn" {
  value = aws_lb_target_group.waiting_room.arn
}
