# Patients Microservice

## GetPatient
Requires a `token` with *admin* role and an `id` of patient. 

Returns a `patient` that corresponds to the given id.

### Errors
- `Unauthenticated` - token is not valid or expired
- `PermissionDenied` - token is not authorized with *admin* role
- `NotFound` - patient with the given id doesn't exist

---

## GetPatientsIds
Requires a `token` with *admin* role, non-negative `skip` and positive `limit` upto 50.

Returns list of patient `ids` in the specified range.

### Errors
- `Unauthenticated` - token is not valid or expired
- `PermissionDenied` - token is not authorized with *admin* role
- `InvalidArgument` - `skip` or `limit` parameters doesn't meet validation requirements

## CreatePatient
Requires a `token` with *admin* role and patient info that at least contains 
nonempty name, personal ID and birthdate.

Returns an `id` of newly created patient.

### Errors
- `Unauthenticated` - token is not valid or expired
- `PermissionDenied` - token is not authorized with *admin* role
- `InvalidArgument` - some required patient info is missed or is malformed

---