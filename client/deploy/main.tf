# CaseWork's infrastructure: one Lightsail instance with a static address and a
# firewall, and the DNS record that points the name at it (client decision 51).
#
# The owner applies this from their own machine. The state stays there and is
# gitignored; credentials come from the environment (AWS_PROFILE or the AWS_*
# variables, and CLOUDFLARE_API_TOKEN) and never appear in these files.
terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }

    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "us-east-2"
}

provider "cloudflare" {}

variable "deploy_public_key" {
  description = "The public half of the key the deploy pipeline logs in with."
  type        = string
}

variable "cloudflare_zone_id" {
  description = "The zone id of happensbefore.com, from the Cloudflare dashboard."
  type        = string
}

# The 4 GB plan. The instance is created with the setup script as its first-boot
# script, and nothing else is done to it by hand: changing the script replaces
# the instance, which is the point of a host that can be rebuilt (decision 55).
resource "aws_lightsail_instance" "casework" {
  name              = "casework"
  availability_zone = "us-east-2a"
  blueprint_id      = "ubuntu_24_04"
  bundle_id         = "medium_3_0"

  user_data = replace(file("${path.module}/setup.sh"), "__DEPLOY_PUBLIC_KEY__", var.deploy_public_key)
}

# A static address, so the name keeps pointing at the host across a restart.
resource "aws_lightsail_static_ip" "casework" {
  name = "casework"
}

resource "aws_lightsail_static_ip_attachment" "casework" {
  static_ip_name = aws_lightsail_static_ip.casework.name
  instance_name  = aws_lightsail_instance.casework.name
}

# Lightsail's own firewall, in front of the host's: 22, 80 and 443.
resource "aws_lightsail_instance_public_ports" "casework" {
  instance_name = aws_lightsail_instance.casework.name

  port_info {
    protocol  = "tcp"
    from_port = 22
    to_port   = 22
  }

  port_info {
    protocol  = "tcp"
    from_port = 80
    to_port   = 80
  }

  port_info {
    protocol  = "tcp"
    from_port = 443
    to_port   = 443
  }
}

# DNS only, not proxied, so the host sees real client addresses (decision 50).
resource "cloudflare_dns_record" "casework" {
  zone_id = var.cloudflare_zone_id
  name    = "casework.happensbefore.com"
  type    = "A"
  content = aws_lightsail_static_ip_attachment.casework.ip_address
  proxied = false
  ttl     = 300
}

output "address" {
  value = aws_lightsail_static_ip_attachment.casework.ip_address
}
