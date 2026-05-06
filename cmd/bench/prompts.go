package main

import "fmt"

// systemPrompt is parameterized by today's date so the model can resolve
// relative dates ("tomorrow", "Monday") deterministically.
func systemPrompt(today string) string {
	return fmt.Sprintf(`You are an appointment booking assistant for "Sunrise Salon & Spa".

Today's date is %s.

We offer these services: haircut, dental_cleaning, massage, manicure, pedicure, facial.

Use the available tools to help users:
- Book new appointments
- Reschedule existing appointments
- Cancel appointments
- Check appointment status
- View available slots
- Get service pricing
- Get business info (hours, location, contact)

Rules:
1. Normalize dates to YYYY-MM-DD format. Resolve relative dates ("today", "tomorrow", "Monday", "next Tuesday") using today's date.
2. Normalize times to 24-hour HH:MM format. "noon" = 12:00, "midnight" = 00:00.
3. Service names must be lowercase with underscores: haircut, dental_cleaning, massage, manicure, pedicure, facial.
4. For new bookings: if the user has provided service, date, time, customer_name, AND customer_phone, call create_appointment directly. Otherwise call check_availability with whatever details are available.
5. If required information is missing, ask ONE clarifying question instead of calling a tool.
6. For greetings, farewells, or out-of-scope questions: reply briefly and do not call any tool.
7. Do not invent appointment IDs. If the user wants to cancel/reschedule but hasn't given an ID, ask for it.
8. Be concise. One short message per turn.`, today)
}

// buildTools returns the tool catalog in OpenAI/Groq function-calling format.
func buildTools() []Tool {
	strProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	enumProp := func(desc string, vals ...string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc, "enum": vals}
	}

	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "check_availability",
				Description: "Check if a service slot is available on a given date and time. Use this BEFORE create_appointment.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"service": strProp("Service name. Empty string if user did not specify."),
						"date":    strProp("Date in YYYY-MM-DD. Empty string if not specified."),
						"time":    strProp("Time in HH:MM 24-hour. Empty string if not specified."),
					},
					"required": []string{"service", "date", "time"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "create_appointment",
				Description: "Create a confirmed appointment. Only call when service, date, time, customer_name, and customer_phone are ALL provided.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"service":        strProp("Service name."),
						"date":           strProp("Date in YYYY-MM-DD."),
						"time":           strProp("Time in HH:MM 24-hour."),
						"customer_name":  strProp("Customer full name."),
						"customer_phone": strProp("Customer phone number, digits only if possible."),
					},
					"required": []string{"service", "date", "time", "customer_name", "customer_phone"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "cancel_appointment",
				Description: "Cancel an existing appointment by ID.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"appointment_id": strProp("The appointment ID provided by the user. Do not invent."),
					},
					"required": []string{"appointment_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "reschedule_appointment",
				Description: "Reschedule an existing appointment to a new date and time.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"appointment_id": strProp("The appointment ID. Do not invent."),
						"new_date":       strProp("New date in YYYY-MM-DD."),
						"new_time":       strProp("New time in HH:MM 24-hour."),
					},
					"required": []string{"appointment_id", "new_date", "new_time"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_appointment_status",
				Description: "Look up the status of an existing appointment.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"appointment_id": strProp("The appointment ID."),
					},
					"required": []string{"appointment_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_services",
				Description: "List all services offered by the business.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
					"required":   []string{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_pricing",
				Description: "Get pricing for a specific service, or all services if service is empty.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"service": strProp("Service name, or empty string for all services."),
					},
					"required": []string{"service"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_business_info",
				Description: "Get business info: hours, location, or contact.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"field": enumProp("Which field to return.", "hours", "location", "contact"),
					},
					"required": []string{"field"},
				},
			},
		},
	}
}
