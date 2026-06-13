module.exports = require("@raycast/eslint-config").flatMap((entry) => (Array.isArray(entry) ? entry : [entry]));
