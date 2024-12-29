
// The keyword to trigger the command
var command = "JS.HASH"

// The string array of categories this command belongs to.
// This array can contain both built-in categories and new custom categories.
var categories = ["hash", "write", "fast"]

// The description of the command.
var description = "(JS.HASH) This is an example of working with SugarDB hashes/maps in js scripts."

// Whether the command should be synced across the RAFT cluster.
var sync = true

/**
 *  keyExtractionFunc is a function that extracts the keys from the command and returns them to SugarDB.keyExtractionFunc
 *  The returned data from this function is used in the Access Control Layer to determine if the current connection is
 *  authorized to execute this command. The function must return a table that specifies which keys in this command
 *  are read keys and which ones are write keys.
 *  Example return: {["readKeys"] = {"key1", "key2"}, ["writeKeys"] = {"key3", "key4", "key5"}}
 *
 *  1. "command" is a string array representing the command that triggered this key extraction function.
 *
 *  2. "args" is a string array of the modifier args that were passed when loading the module into SugarDB.
 *  These args are passed to the key extraction function everytime it's invoked.
 */
function keyExtractionFunc(command, args) {
  if (command.length !== 3) {
    throw "wrong number of args, expected 2."
  }
  return {
    "readKeys": [],
    "writeKeys": [command[1], command[2]]
  }
}

/**
 * handlerFunc is the command's handler function. The function is passed some arguments that allow it to interact with
 * SugarDB. The function must return a valid RESP response or throw an error.
 * The handler function accepts the following args:
 *
 * 1. "context" is a table that contains some information about the environment this command has been executed in.
 *     Example: {["protocol"] = 2, ["database"] = 0}
 *     This object contains the following properties:
 *     i) protocol - the protocol version of the client that executed the command (either 2 or 3).
 *     ii) database - the active database index of the client that executed the command.
 *
 * 2. "command" is the string array representing the command that triggered this handler function.
 *
 * 3. "keyExists" is a function that can be called to check if a list of keys exists in the SugarDB store database.
 *     This function accepts a string array of keys to check and returns a table with each key having a corresponding
 *     boolean value indicating whether it exists.
 *     Examples:
 *     i) Example invocation: keyExists({"key1", "key2", "key3"})
 *     ii) Example return: {["key1"] = true, ["key2"] = false, ["key3"] = true}
 *
 * 4. "getValues" is a function that can be called to retrieve values from the SugarDB store database.
 *     The function accepts a string array of keys whose values we would like to fetch, and returns a table with each key
 *     containing the corresponding value from the store.
 *     The possible data types for the values are: number, string, nil, hash, set, zset
 *     Examples:
 *     i) Example invocation: getValues({"key1", "key2", "key3"})
 *     ii) Example return: {["key1"] = 3.142, ["key2"] = nil, ["key3"] = "Pi"}
 *
 * 5. "setValues" is a function that can be called to set values in the active database in the SugarDB store.
 *     This function accepts a table with keys and the corresponding values to set for each key in the active database
 *     in the store.
 *     The accepted data types for the values are: number, string, nil, hash, set, zset.
 *     The setValues function does not return anything.
 *     Examples:
 *     i) Example invocation: setValues({["key1"] = 3.142, ["key2"] = nil, ["key3"] = "Pi"})
 *
 * 6. "args" is a string array of the modifier args passed to the module at load time. These args are passed to the
 *    handler everytime it's invoked.
 */
function handlerFunc(ctx, command, keysExist, getValues, setValues, args) {
  var key1 = command[1]
  var key2 = command[2]

  var hash1 = createHash()
  var count = hash1.set({"a": "1", "b": "2"})
  console.log("HASH1 COUNT (SET): " + count)
  count = hash1.setnx({"b": "3", "c": "3", "d": "4", "e": "5"})
  console.log("HASH1 COUNT (SETNX): " + count)
  var values = hash1.get("a", "b", "c")
  for (var key in values) {
    console.log("HASH1 (GET): " + key + " " + values[key])
  }
  var length = hash1.length()
  console.log("HASH1 (LENGTH): " + length)
  count = hash1.delete("a", "b", "x", "y", "z")
  console.log("HASH1 COUNT (DELETE): " + count)
  values = hash1.all()
  for (var key in values) {
    console.log("HASH1 (ALL): " + key + " " + values[key])
  }
  var exists = hash1.exists("a", "b", "c", "d", "e")
  for (var key in exists) {
    console.log("HASH1 (EXISTS): " + key + " " + exists[key])
  }

  var hash2 = createHash({"a": "1", "b": "2", "c": "3"})

  setValues({ key1: hash1, key2: hash2, key3: 100, key4: "value4" })

  return "+OK\r\n"
}