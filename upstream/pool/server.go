package pool

type Server struct {
	Name     string
	Address  string
	Port     string
	Priority uint
}

type ByPriority []Server

func (servers ByPriority) Len() int {
	return len(servers)
}

func (servers ByPriority) Swap(i, j int) {
	servers[i], servers[j] = servers[j], servers[i]
}

func (servers ByPriority) Less(i, j int) bool {
	return servers[i].Priority < servers[j].Priority
}
