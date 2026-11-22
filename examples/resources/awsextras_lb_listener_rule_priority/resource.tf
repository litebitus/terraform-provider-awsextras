resource "awsextras_lb_listener_rule_priority" "mock" {
  listener_arn = aws_lb_listener.mock.arn
  min_priority = 1000
  max_priority = 2000
}

resource "aws_lb_listener_rule" "mock" {
  listener_arn = aws_lb_listener.mock.arn
  priority     = awsextras_lb_listener_rule_priority.mock.priority
  tags         = awsextras_lb_listener_rule_priority.mock.priority_tag

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.mock.arn
  }

  condition {
    host_header {
      values = ["mock.example.com"]
    }
  }
}
