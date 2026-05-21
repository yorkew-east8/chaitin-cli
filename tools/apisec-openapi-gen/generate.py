#!/usr/bin/env python3
import argparse
import ast
import json
from pathlib import Path


HTTP_METHODS = {"get", "post", "put", "delete", "patch"}


def main():
    parser = argparse.ArgumentParser(description="Generate APISec management OpenAPI from skyview source")
    parser.add_argument("--source", required=True, help="Path to a-vatar/skyview/skyview")
    parser.add_argument("--output", required=True, help="Output openapi.json path")
    args = parser.parse_args()

    source = Path(args.source)
    serializers = load_serializers(source)
    paths = {}
    tags = set()

    for view_file in sorted(source.glob("*/views.py")):
        module = view_file.parent.name
        tags.add(module)
        tree = parse_file(view_file)
        class_nodes = {node.name: node for node in tree.body if isinstance(node, ast.ClassDef)}
        for cls in [node for node in tree.body if isinstance(node, ast.ClassDef) and is_exported_api_view(node, class_nodes)]:
            methods = collect_methods(cls, class_nodes)
            if not methods:
                continue
            path = f"/api/{cls.name}"
            item = paths.setdefault(path, {})
            for method in methods:
                serializer_name, many = serializer_for_method(method)
                operation = {
                    "operationId": f"{cls.name}_{method.name}",
                    "summary": summary_for(cls.name, method.name),
                    "tags": [module],
                    "parameters": [],
                    "x-cli-body-fallback": True,
                }
                schema = serializers.get(serializer_name or "")
                if schema:
                    operation["x-cli-body-fallback"] = False
                    if many:
                        schema = {"type": "array", "items": schema}
                    if method.name == "get":
                        operation["parameters"] = parameters_from_schema(schema)
                    else:
                        operation["requestBody"] = {
                            "required": True,
                            "content": {"application/json": {"schema": schema}},
                        }
                elif method.name != "get":
                    operation["requestBody"] = {
                        "required": False,
                        "content": {"application/json": {"schema": {"type": "object"}}},
                    }
                item[method.name] = operation

    openapi = {
        "openapi": "3.0.3",
        "info": {
            "title": "APISec Management API",
            "version": "26.05",
            "description": "Generated from APISec skyview APIView classes.",
        },
        "tags": [{"name": tag} for tag in sorted(tags)],
        "paths": dict(sorted(paths.items())),
    }

    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(openapi, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def parse_file(path):
    return ast.parse(path.read_text(encoding="utf-8"), filename=str(path))


def load_serializers(source):
    serializers = {}
    for serializer_file in sorted(source.glob("*/serializers.py")):
        tree = parse_file(serializer_file)
        class_nodes = {node.name: node for node in tree.body if isinstance(node, ast.ClassDef)}
        pending = set(class_nodes)
        while pending:
            progressed = False
            for name in list(pending):
                node = class_nodes[name]
                bases = [base_name(base) for base in node.bases]
                if any(base in class_nodes and base not in serializers for base in bases):
                    continue
                schema = {"type": "object", "properties": {}, "required": []}
                for base in bases:
                    if base in serializers:
                        merge_schema(schema, serializers[base])
                for stmt in node.body:
                    if isinstance(stmt, ast.Assign):
                        field_name = assigned_name(stmt)
                        if not field_name:
                            continue
                        field_schema, required = schema_for_field(stmt.value)
                        if field_schema is None:
                            continue
                        schema["properties"][field_name] = field_schema
                        if required:
                            schema["required"].append(field_name)
                if not schema["required"]:
                    schema.pop("required")
                serializers[name] = schema
                pending.remove(name)
                progressed = True
            if not progressed:
                break
    return serializers


def merge_schema(target, source):
    target["properties"].update(source.get("properties", {}))
    required = set(target.get("required", []))
    required.update(source.get("required", []))
    target["required"] = sorted(required)


def collect_methods(cls, class_nodes):
    methods = {}

    def visit(node):
        for base in node.bases:
            base = base_name(base)
            if base in class_nodes:
                visit(class_nodes[base])
        for item in node.body:
            if isinstance(item, ast.FunctionDef) and item.name in HTTP_METHODS:
                methods[item.name] = item

    visit(cls)
    return [methods[name] for name in sorted(methods, key=lambda item: ["get", "post", "put", "delete", "patch"].index(item))]


def is_exported_api_view(cls, class_nodes):
    if class_has_true_assignment(cls, "__is_abstract"):
        return False
    if cls.name.endswith("API"):
        return True

    seen = set()

    def inherits_api_view(node):
        if node.name in seen:
            return False
        seen.add(node.name)
        for base in node.bases:
            name = base_name(base)
            if name in {"APIView", "CSRFExemptAPIView"}:
                return True
            if name in class_nodes and inherits_api_view(class_nodes[name]):
                return True
        return False

    return inherits_api_view(cls)


def class_has_true_assignment(cls, name):
    for stmt in cls.body:
        if not isinstance(stmt, ast.Assign):
            continue
        for target in stmt.targets:
            if isinstance(target, ast.Name) and target.id == name and literal_bool(stmt.value):
                return True
    return False


def serializer_for_method(method):
    for decorator in method.decorator_list:
        call = decorator if isinstance(decorator, ast.Call) else None
        if call is None or call_name(call.func) != "serialize":
            continue
        serializer_name = None
        if call.args:
            serializer_name = call_name(call.args[0])
        many = any(keyword.arg == "serializer_many" and literal_bool(keyword.value) for keyword in call.keywords)
        return serializer_name, many
    return None, False


def parameters_from_schema(schema):
    params = []
    required = set(schema.get("required", []))
    for name, prop in sorted(schema.get("properties", {}).items()):
        params.append({
            "name": name,
            "in": "query",
            "required": name in required,
            "description": prop.get("description", ""),
            "schema": prop,
        })
    return params


def schema_for_field(value):
    call = value if isinstance(value, ast.Call) else None
    if call is None:
        return None, False
    name = call_name(call.func)
    if not name:
        return None, False
    required = keyword_value(call, "required")
    required = True if required is None else bool(required)
    help_text = keyword_value(call, "help_text")
    schema = {"type": field_type(name)}
    if help_text:
        schema["description"] = str(help_text)
    if name.endswith("ListField") or name.endswith("ListSerializer"):
        schema["type"] = "array"
        schema["items"] = {"type": "string"}
    if name.endswith("JSONField") or name.endswith("DictField"):
        schema["type"] = "object"
    return schema, required


def field_type(name):
    if name.endswith("IntegerField"):
        return "integer"
    if name.endswith("BooleanField"):
        return "boolean"
    if name.endswith("FloatField"):
        return "number"
    if name.endswith("JSONField") or name.endswith("DictField"):
        return "object"
    return "string"


def assigned_name(stmt):
    if len(stmt.targets) != 1:
        return None
    target = stmt.targets[0]
    return target.id if isinstance(target, ast.Name) else None


def keyword_value(call, name):
    for keyword in call.keywords:
        if keyword.arg == name:
            try:
                return ast.literal_eval(keyword.value)
            except Exception:
                return None
    return None


def literal_bool(node):
    try:
        return bool(ast.literal_eval(node))
    except Exception:
        return False


def base_name(node):
    return call_name(node)


def call_name(node):
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        parent = call_name(node.value)
        return f"{parent}.{node.attr}" if parent else node.attr
    if isinstance(node, ast.Call):
        return call_name(node.func)
    return None


def summary_for(class_name, method):
    return f"{method.upper()} /api/{class_name}"


if __name__ == "__main__":
    main()
