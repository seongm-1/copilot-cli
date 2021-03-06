// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ec2 provides a client to make API requests to Amazon Elastic Compute Cloud.
package ec2

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	defaultForAZFilterName = "default-for-az"

	// TagFilterName is the filter name format for tag filters
	TagFilterName = "tag:%s"
)

// ListVPCSubnetsOpts sets up optional parameters for ListVPCSubnets function.
type ListVPCSubnetsOpts func([]*ec2.Subnet) []*ec2.Subnet

// FilterForPublicSubnets is used to filter to get public subnets.
func FilterForPublicSubnets() ListVPCSubnetsOpts {
	return func(subnets []*ec2.Subnet) []*ec2.Subnet {
		var publicSubnets []*ec2.Subnet
		for _, subnet := range subnets {
			if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
				publicSubnets = append(publicSubnets, subnet)
			}
		}
		return publicSubnets
	}
}

// FilterForPrivateSubnets is used to filter to get private subnets.
func FilterForPrivateSubnets() ListVPCSubnetsOpts {
	return func(subnets []*ec2.Subnet) []*ec2.Subnet {
		var privateSubnets []*ec2.Subnet
		for _, subnet := range subnets {
			if !aws.BoolValue(subnet.MapPublicIpOnLaunch) {
				privateSubnets = append(privateSubnets, subnet)
			}
		}
		return privateSubnets
	}
}

var (
	// FilterForDefaultVPCSubnets is a pre-defined filter for the default subnets at the availability zone.
	FilterForDefaultVPCSubnets = Filter{
		Name:   defaultForAZFilterName,
		Values: []string{"true"},
	}
)

type api interface {
	DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
}

// Filter contains the name and values of a filter.
type Filter struct {
	// Name of a filter that will be applied to subnets,
	// for available filter names see: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSubnets.html.
	Name string
	// Value of the filter.
	Values []string
}

// EC2 wraps an AWS EC2 client.
type EC2 struct {
	client api
}

// New returns a EC2 configured against the input session.
func New(s *session.Session) *EC2 {
	return &EC2{
		client: ec2.New(s),
	}
}

// ListVPC returns IDs of all VPCs.
func (c *EC2) ListVPC() ([]string, error) {
	var vpcs []*ec2.Vpc
	response, err := c.client.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe VPCs: %w", err)
	}
	vpcs = append(vpcs, response.Vpcs...)

	for response.NextToken != nil {
		response, err = c.client.DescribeVpcs(&ec2.DescribeVpcsInput{
			NextToken: response.NextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe VPCs: %w", err)
		}
		vpcs = append(vpcs, response.Vpcs...)
	}
	var vpcNames []string
	for _, vpc := range vpcs {
		vpcNames = append(vpcNames, aws.StringValue(vpc.VpcId))
	}

	return vpcNames, nil
}

// ListVPCSubnets lists all subnets given a VPC ID.
func (c *EC2) ListVPCSubnets(vpcID string, opts ...ListVPCSubnetsOpts) ([]string, error) {
	respSubnets, err := c.subnets(Filter{
		Name:   "vpc-id",
		Values: []string{vpcID},
	})
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		respSubnets = opt(respSubnets)
	}
	var subnets []string
	for _, subnet := range respSubnets {
		subnets = append(subnets, aws.StringValue(subnet.SubnetId))
	}
	return subnets, nil
}

// SubnetIDs finds the subnet IDs with optional filters.
func (c *EC2) SubnetIDs(filters ...Filter) ([]string, error) {
	subnets, err := c.subnets(filters...)
	if err != nil {
		return nil, err
	}

	subnetIDs := make([]string, len(subnets))
	for idx, subnet := range subnets {
		subnetIDs[idx] = aws.StringValue(subnet.SubnetId)
	}
	return subnetIDs, nil
}

// PublicSubnetIDs finds the public subnet IDs with optional filters.
func (c *EC2) PublicSubnetIDs(filters ...Filter) ([]string, error) {
	subnets, err := c.subnets(filters...)
	if err != nil {
		return nil, err
	}

	var subnetIDs []string
	for _, subnet := range subnets {
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
			subnetIDs = append(subnetIDs, aws.StringValue(subnet.SubnetId))
		}
	}
	return subnetIDs, nil
}

// SecurityGroups finds the security group IDs with optional filters.
func (c *EC2) SecurityGroups(filters ...Filter) ([]string, error) {
	inputFilters := toEC2Filter(filters)

	response, err := c.client.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: inputFilters,
	})

	if err != nil {
		return nil, fmt.Errorf("describe security groups: %w", err)
	}

	securityGroups := make([]string, len(response.SecurityGroups))
	for idx, sg := range response.SecurityGroups {
		securityGroups[idx] = aws.StringValue(sg.GroupId)
	}
	return securityGroups, nil
}

func (c *EC2) subnets(filters ...Filter) ([]*ec2.Subnet, error) {
	inputFilters := toEC2Filter(filters)
	var subnets []*ec2.Subnet
	response, err := c.client.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: inputFilters,
	})
	if err != nil {
		return nil, fmt.Errorf("describe subnets: %w", err)
	}
	subnets = append(subnets, response.Subnets...)

	for response.NextToken != nil {
		response, err = c.client.DescribeSubnets(&ec2.DescribeSubnetsInput{
			Filters:   inputFilters,
			NextToken: response.NextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe subnets: %w", err)
		}
		subnets = append(subnets, response.Subnets...)
	}

	return subnets, nil
}

func toEC2Filter(filters []Filter) []*ec2.Filter {
	var ec2Filter []*ec2.Filter
	for _, filter := range filters {
		ec2Filter = append(ec2Filter, &ec2.Filter{
			Name:   aws.String(filter.Name),
			Values: aws.StringSlice(filter.Values),
		})
	}
	return ec2Filter
}
