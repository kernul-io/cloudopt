package awsmetrics

import (
	"github.com/kernul-io/cloudopt/internal/domain"
)

type metricSpec struct {
	Namespace  string
	Name       string
	Statistic  string
	Unit       string
	Dimension  string
	DimValueFn func(domain.Resource) string
}

func specsForResource(r domain.Resource) []metricSpec {
	switch r.Kind {
	case domain.KindComputeInstance:
		return []metricSpec{
			{Namespace: "AWS/EC2", Name: "CPUUtilization", Statistic: "Average", Unit: "Percent", Dimension: "InstanceId", DimValueFn: providerID},
			{Namespace: "AWS/EC2", Name: "NetworkIn", Statistic: "Sum", Unit: "Bytes", Dimension: "InstanceId", DimValueFn: providerID},
			{Namespace: "AWS/EC2", Name: "NetworkOut", Statistic: "Sum", Unit: "Bytes", Dimension: "InstanceId", DimValueFn: providerID},
			{Namespace: "AWS/EC2", Name: "DiskReadOps", Statistic: "Sum", Unit: "Count", Dimension: "InstanceId", DimValueFn: providerID},
			{Namespace: "AWS/EC2", Name: "DiskWriteOps", Statistic: "Sum", Unit: "Count", Dimension: "InstanceId", DimValueFn: providerID},
			{Namespace: "AWS/EC2", Name: "StatusCheckFailed", Statistic: "Maximum", Unit: "Count", Dimension: "InstanceId", DimValueFn: providerID},
		}
	case domain.KindBlockVolume:
		return []metricSpec{
			{Namespace: "AWS/EBS", Name: "VolumeReadBytes", Statistic: "Sum", Unit: "Bytes", Dimension: "VolumeId", DimValueFn: providerID},
			{Namespace: "AWS/EBS", Name: "VolumeWriteBytes", Statistic: "Sum", Unit: "Bytes", Dimension: "VolumeId", DimValueFn: providerID},
			{Namespace: "AWS/EBS", Name: "VolumeReadOps", Statistic: "Sum", Unit: "Count", Dimension: "VolumeId", DimValueFn: providerID},
			{Namespace: "AWS/EBS", Name: "VolumeWriteOps", Statistic: "Sum", Unit: "Count", Dimension: "VolumeId", DimValueFn: providerID},
			{Namespace: "AWS/EBS", Name: "VolumeQueueLength", Statistic: "Average", Unit: "Count", Dimension: "VolumeId", DimValueFn: providerID},
			{Namespace: "AWS/EBS", Name: "BurstBalance", Statistic: "Average", Unit: "Percent", Dimension: "VolumeId", DimValueFn: providerID},
		}
	case domain.KindDatabase:
		return []metricSpec{
			{Namespace: "AWS/RDS", Name: "CPUUtilization", Statistic: "Average", Unit: "Percent", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "DatabaseConnections", Statistic: "Average", Unit: "Count", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "FreeStorageSpace", Statistic: "Average", Unit: "Bytes", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "ReadIOPS", Statistic: "Average", Unit: "Count/Second", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "WriteIOPS", Statistic: "Average", Unit: "Count/Second", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "ReadLatency", Statistic: "Average", Unit: "Seconds", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "WriteLatency", Statistic: "Average", Unit: "Seconds", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
			{Namespace: "AWS/RDS", Name: "FreeableMemory", Statistic: "Average", Unit: "Bytes", Dimension: "DBInstanceIdentifier", DimValueFn: providerID},
		}
	case domain.KindNATGateway:
		return []metricSpec{
			{Namespace: "AWS/NATGateway", Name: "BytesInFromDestination", Statistic: "Sum", Unit: "Bytes", Dimension: "NatGatewayId", DimValueFn: providerID},
			{Namespace: "AWS/NATGateway", Name: "BytesOutToDestination", Statistic: "Sum", Unit: "Bytes", Dimension: "NatGatewayId", DimValueFn: providerID},
			{Namespace: "AWS/NATGateway", Name: "ActiveConnectionCount", Statistic: "Average", Unit: "Count", Dimension: "NatGatewayId", DimValueFn: providerID},
			{Namespace: "AWS/NATGateway", Name: "PacketsDropCount", Statistic: "Sum", Unit: "Count", Dimension: "NatGatewayId", DimValueFn: providerID},
			{Namespace: "AWS/NATGateway", Name: "ErrorPortAllocation", Statistic: "Sum", Unit: "Count", Dimension: "NatGatewayId", DimValueFn: providerID},
		}
	default:
		return nil
	}
}

func providerID(r domain.Resource) string {
	return r.ProviderResourceID
}
