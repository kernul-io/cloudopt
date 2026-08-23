package gcpinventory

// Adapter-internal shapes (not domain types). Live and fixture backends emit these.

type CallerIdentity struct {
	Email     string `json:"email"`
	UniqueID  string `json:"unique_id"`
	ProjectID string `json:"project_id"`
	Principal string `json:"principal"`
}

type Project struct {
	ProjectID     string `json:"project_id"`
	DisplayName   string `json:"display_name"`
	Parent        string `json:"parent"`
	ProjectNumber string `json:"project_number"`
}

type ProjectInventory struct {
	ProjectID string            `json:"project_id"`
	Regions   []RegionInventory `json:"regions"`
	Global    GlobalInventory   `json:"global"`
}

type RegionInventory struct {
	Region string          `json:"region"`
	Zones  []ZoneInventory `json:"zones"`
}

type ZoneInventory struct {
	Zone string `json:"zone"`
}

type GlobalInventory struct {
	Networks        []Network        `json:"networks"`
	Images          []Image          `json:"images"`
	ForwardingRules []ForwardingRule `json:"forwarding_rules"`
}

type ScopedInventory struct {
	Networks        []Network        `json:"networks"`
	Instances       []Instance       `json:"instances"`
	Disks           []Disk           `json:"disks"`
	Snapshots       []Snapshot       `json:"snapshots"`
	Images          []Image          `json:"images"`
	MachineTypes    []MachineType    `json:"machine_types"`
	Subnets         []Subnet         `json:"subnets"`
	Routes          []Route          `json:"routes"`
	Addresses       []Address        `json:"addresses"`
	CloudNAT        []CloudNAT       `json:"cloud_nat"`
	ForwardingRules []ForwardingRule `json:"forwarding_rules"`
	SQLInstances    []SQLInstance    `json:"sql_instances"`
	GKEClusters     []GKECluster     `json:"gke_clusters"`
}

type Network struct {
	SelfLink          string `json:"self_link"`
	Name              string `json:"name"`
	ProjectID         string `json:"project_id"`
	AutoCreateSubnets bool   `json:"auto_create_subnets"`
}

type Subnet struct {
	SelfLink  string `json:"self_link"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	Region    string `json:"region"`
	Network   string `json:"network"`
	CIDR      string `json:"cidr"`
}

type Route struct {
	SelfLink  string `json:"self_link"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	Network   string `json:"network"`
	DestRange string `json:"dest_range"`
	NextHop   string `json:"next_hop"`
	Priority  int64  `json:"priority"`
}

type ForwardingRule struct {
	SelfLink            string `json:"self_link"`
	Name                string `json:"name"`
	ProjectID           string `json:"project_id"`
	Region              string `json:"region"`
	IPAddress           string `json:"ip_address"`
	Target              string `json:"target"`
	LoadBalancingScheme string `json:"load_balancing_scheme"`
}

type Address struct {
	SelfLink  string   `json:"self_link"`
	Name      string   `json:"name"`
	ProjectID string   `json:"project_id"`
	Region    string   `json:"region"`
	Address   string   `json:"address"`
	Status    string   `json:"status"`
	Users     []string `json:"users"`
}

type CloudNAT struct {
	SelfLink  string `json:"self_link"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	Region    string `json:"region"`
	Network   string `json:"network"`
	Router    string `json:"router"`
}

type Instance struct {
	SelfLink    string            `json:"self_link"`
	Name        string            `json:"name"`
	ProjectID   string            `json:"project_id"`
	Zone        string            `json:"zone"`
	Region      string            `json:"region"`
	MachineType string            `json:"machine_type"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Disks       []InstanceDisk    `json:"disks"`
	Network     string            `json:"network"`
	Subnetwork  string            `json:"subnetwork"`
}

type InstanceDisk struct {
	DeviceName string `json:"device_name"`
	Source     string `json:"source"`
	Boot       bool   `json:"boot"`
}

type Disk struct {
	SelfLink  string            `json:"self_link"`
	Name      string            `json:"name"`
	ProjectID string            `json:"project_id"`
	Zone      string            `json:"zone"`
	Region    string            `json:"region"`
	SizeGB    int64             `json:"size_gb"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	Users     []string          `json:"users"`
	Labels    map[string]string `json:"labels"`
}

type Snapshot struct {
	SelfLink   string            `json:"self_link"`
	Name       string            `json:"name"`
	ProjectID  string            `json:"project_id"`
	SourceDisk string            `json:"source_disk"`
	Status     string            `json:"status"`
	Labels     map[string]string `json:"labels"`
}

type Image struct {
	SelfLink  string            `json:"self_link"`
	Name      string            `json:"name"`
	ProjectID string            `json:"project_id"`
	Family    string            `json:"family"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels"`
}

type MachineType struct {
	SelfLink  string `json:"self_link"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	Zone      string `json:"zone"`
	Region    string `json:"region"`
	VCPUs     int64  `json:"vcpu"`
	MemoryMB  int64  `json:"memory_mb"`
	Family    string `json:"family"`
}

type SQLInstance struct {
	SelfLink        string            `json:"self_link"`
	Name            string            `json:"name"`
	ProjectID       string            `json:"project_id"`
	Region          string            `json:"region"`
	DatabaseVersion string            `json:"database_version"`
	State           string            `json:"state"`
	Tier            string            `json:"tier"`
	Labels          map[string]string `json:"labels"`
}

type GKECluster struct {
	SelfLink   string            `json:"self_link"`
	Name       string            `json:"name"`
	ProjectID  string            `json:"project_id"`
	Location   string            `json:"location"`
	Status     string            `json:"status"`
	Network    string            `json:"network"`
	Subnetwork string            `json:"subnetwork"`
	NodePools  []GKENodePool     `json:"node_pools"`
	Labels     map[string]string `json:"labels"`
}

type GKENodePool struct {
	SelfLink    string `json:"self_link"`
	Name        string `json:"name"`
	MachineType string `json:"machine_type"`
	Status      string `json:"status"`
	NodeCount   int64  `json:"node_count"`
}
