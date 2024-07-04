## gRPC Functions

### GetPatient

Retrieves the details of a specific patient by their ID.

**Request:**

```protobuf
message GetPatientRequest {
  string token = 1; // Authentication token
  int32 id = 2; // ID of the patient
}
```

**Response:**

```protobuf
message GetPatientResponse {
  Patient patient = 1; // Details of the patient
}
```

**Errors:**

- `Unauthenticated` - Token is not valid or expired.
- `PermissionDenied` - Token is not authorized with the *admin* role.
- `NotFound` - Patient with the given ID does not exist.

---

### GetPatientsIDs

Retrieves a list of patient IDs with pagination support.

**Request:**

```protobuf
message GetPatientsIDsRequest {
  string token = 1; // Authentication token
  int32 limit = 2; // Maximum number of results to return
  int32 offset = 3; // Offset for pagination
}
```

**Response:**

```protobuf
message GetPatientsIDsResponse {
  int32 count = 1; // Total number of patients
  repeated int32 results = 2; // List of patient IDs
}
```

**Errors:**

- `Unauthenticated` - Token is not valid or expired.
- `PermissionDenied` - Token is not authorized with the *admin* role.
- `InvalidArgument` - `offset` or `limit` parameters are invalid.

---

### CreatePatient

Creates a new patient record with the provided details.

**Request:**

```protobuf
message CreatePatientRequest {
  string token = 1; // Authentication token
  string name = 2; // Name of the patient
  Patient.PersonalID personal_id = 3; // Personal ID of the patient
  Patient.Gender gender = 4; // Gender of the patient (optional)
  string phone_number = 5; // Phone number of the patient (optional)
  repeated string languages = 6; // Languages spoken by the patient (optional)
  string birth_date = 7; // Birth date of the patient
  repeated Patient.EmergencyContact emergency_contacts = 8; // Emergency contacts of the patient (optional)
  string referred_by = 9; // Who referred the patient (optional)
  string special_note = 10; // Special notes regarding the patient (optional)
}
```

**Response:**

```protobuf
message CreatePatientResponse {
  int32 id = 1; // ID of the newly created patient
}
```

**Errors:**

- `Unauthenticated` - Token is not valid or expired.
- `PermissionDenied` - Token is not authorized with the *admin* role.
- `InvalidArgument` - Required patient information is missing or malformed.

---

## Model Definition

```protobuf
message Patient {
  int32 id = 1; // ID of the patient
  bool active = 2; // Flag indicating if the patient is active
  string name = 3; // Name of the patient
  // Details of the patient
  message PersonalID {
    string id = 1; // Personal ID of the patient
    string type = 2; // Type of personal ID
  }
  enum Gender {
    UNSPECIFIED = 0;
    MALE = 1;
    FEMALE = 2;
  }
  message EmergencyContact {
    string name = 1; // Name of the emergency contact
    string closeness = 2; // Relationship closeness
    string phone = 3; // Phone number of the emergency contact
  }
  PersonalID personal_id = 4; // Personal ID of the patient
  Gender gender = 5; // Gender of the patient
  string phone_number = 6; // Phone number of the patient
  repeated string languages = 7; // Languages spoken by the patient
  string birth_date = 8; // Birth date of the patient
  int32 age = 9; // Age of the patient
  string referred_by = 10; // Who referred the patient
  repeated EmergencyContact emergency_contacts = 11; // Emergency contacts of the patient
  string special_note = 12; // Special notes regarding the patient
}
```