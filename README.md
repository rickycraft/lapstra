# Lapstra

Lapstra is a Go command-line tool for managing Juniper Apstra blueprints. It provides an interactive terminal menu and dedicated operations for virtual networks, connectivity templates, routing zones, configuration deployment, and CSV workflows.

## Motivation

Managing the same network services across multiple blueprints involves repeated configuration steps. Lapstra brings these tasks into a single workflow, with shared VLAN-to-VNI mapping across datacenter pairs and CSV input for bulk changes.

# Architecture

| Path | Purpose |
| --- | --- |
| `cli/` | Implements the `-o` operations, including CSV parsing, validation, imports, exports, and template rendering. |
| `cmd/` | Implements interactive commands, terminal forms, blueprint selection, and the shared `LapstraCmd` session. |
| `core/` | Provides shared Apstra API and domain logic for virtual networks, connectivity templates, routing zones, configlets, commits, device context, CSV files, and logging. |
| `templates/` | Contains Jinja-style `.j2` templates used by the `Templates` operation to render device configuration. |

# Commands

Start the interactive menu with:

```sh
lapstra -a https://your-apstra-server -u your-username
```

## Interactive commands

Commands operate on the blueprints selected at startup.

| Command | Description |
| --- | --- |
| `VlanSearch` | Finds a virtual network by VLAN ID and lists its connectivity templates. |
| `VlanCreateL2` | Creates an L2 virtual network with VLAN bindings and a mapped VNI. |
| `VlanCreateL3` | Creates an L3 virtual network with an anycast gateway and routing zone. |
| `TransformVlanL3` | Adds an anycast gateway and routing zone to an existing virtual network selected by VNI. |
| `VniDelete` | Deletes the virtual network matching a VNI after confirmation. |
| `CtVlanSearch` | Lists the virtual networks in a selected multi-VLAN connectivity template. |
| `CtMultiVlanCreate` | Creates an empty multi-VLAN connectivity template. |
| `CtVxlanAdd` | Adds virtual networks identified by VNI to a multi-VLAN connectivity template. |
| `CtVxlanRemove` | Removes a selected VNI's virtual network from a multi-VLAN connectivity template. |
| `CtVxlanSearch` | Finds multi-VLAN connectivity templates containing a given VNI. |
| `CtStaticRouteAdd` | Adds a static route and next hop in a selected routing zone to a connectivity template. |
| `AddStaticRouteCSV` | Imports static routes from CSV, creating missing connectivity templates and assigning them to both leaves. |
| `RoutingZoneCreate` | Creates a routing zone (VRF). |
| `ConfigletReimport` | Refreshes configlets matching a selected label from the Apstra catalog. |
| `CheckConfig` | Runs configuration commit checks and waits for the results. |
| `Deploy` | Shows configuration differences and starts deployment after confirmation. |
| `Revert` | Discards uncommitted blueprint changes after confirmation. |
| `Exit` | Closes the application. |

## Operations (`-o`)

Select an operation by its case-sensitive name. Operations can still prompt for input.

```sh
lapstra -a https://your-apstra-server -u your-username -o VlanCreate vlans.csv
```

| Operation | Description |
| --- | --- |
| `VniMerge` | Reads CSV rows to align each VLAN's VNI with a master site, optionally removing the VLAN from the `L2 dc-` connectivity template. |
| `CtValidate` | Lists visible connectivity templates whose exported policy lacks `user_data`. |
| `VlanCreate` | Creates L2 or L3 virtual networks from CSV across selected blueprints. |
| `CtExport` | Exports tagged memberships of simple multi-VLAN connectivity templates from one blueprint to `<blueprint>-l2-ct.csv`. |
| `CtImport` | Creates or replaces multi-VLAN connectivity templates in one blueprint from CSV. |
| `Templates` | Selects one blueprint and leaf, then renders `.j2` templates using the leaf's configuration context and prints the results. |

`VniMerge` and `VlanCreate` take a CSV path as the final argument. `CtImport` and the interactive `AddStaticRouteCSV` command let you select a CSV file from the current directory.

