package functiontools

func listTools() ([]Description, error) {
	descriptions := []Description{}
	for name, tool := range registry {
		if !tool.isExported {
			continue
		}
		parameters := []string{}
		for parameter := range tool.properties {
			parameters = append(parameters, parameter)
		}
		descriptions = append(descriptions, Description{
			Type:        "function",
			Name:        name,
			Description: tool.description,
			Strict:      true,
			Parameters: Parameters{
				Type:                 "object",
				Properties:           tool.properties,
				Required:             parameters,
				AdditionalProperties: false,
			},
		})
	}
	return descriptions, nil
}
