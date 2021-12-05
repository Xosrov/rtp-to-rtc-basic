// authenticate user before specific connections
package main

func auth_room_code(room_code string) bool {
	// return true if valid code
	if room_code == "exampletoken" {
		return true
	}
	return false
}
