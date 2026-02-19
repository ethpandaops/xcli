/**
 * @fileoverview Ban hardcoded colors (hex, rgb, hsl) in Tailwind classes and inline styles
 *
 * This rule prevents developers from using hardcoded color values in Tailwind
 * className strings and React inline styles. All colors should come from the
 * semantic color tokens defined in src/index.css.
 *
 * Examples of incorrect code:
 *   className="bg-[#ff0000]"
 *   className="text-[rgb(255,0,0)]"
 *   style={{ color: '#ff0000' }}
 *
 * Examples of correct code:
 *   className="bg-primary"
 *   className="text-danger"
 *   style={{ color: 'var(--color-primary)' }}
 */

/** @type {import('eslint').Rule.RuleModule} */
module.exports = {
  meta: {
    type: 'problem',
    docs: {
      description: 'Disallow hardcoded colors in Tailwind classes',
      category: 'Best Practices',
      recommended: true,
    },
    messages: {
      hardcodedHexColor:
        'Hardcoded hex color "{{value}}" detected. Use semantic color tokens from src/index.css instead (e.g., bg-primary, var(--color-primary)).',
      hardcodedRgbColor:
        'Hardcoded RGB color "{{value}}" detected. Use semantic color tokens from src/index.css instead (e.g., bg-primary, var(--color-primary)).',
      hardcodedHslColor:
        'Hardcoded HSL color "{{value}}" detected. Use semantic color tokens from src/index.css instead (e.g., bg-primary, var(--color-primary)).',
      hardcodedNamedColor:
        'Hardcoded named color "{{value}}" detected in {{property}}. Use semantic color tokens from src/index.css instead (e.g., var(--color-primary)).',
    },
    schema: [],
  },

  create(context) {
    const hexPattern = /\[#[0-9a-fA-F]{3,8}\]/g;
    const rgbPattern = /\[rgba?\([^)]+\)\]/g;
    const hslPattern = /\[hsla?\([^)]+\)\]/g;

    const hexValuePattern = /^#[0-9a-fA-F]{3,8}$/;
    const rgbValuePattern = /^rgba?\([^)]+\)$/;
    const hslValuePattern = /^hsla?\([^)]+\)$/;

    const namedColors = new Set([
      'black',
      'white',
      'red',
      'green',
      'blue',
      'yellow',
      'orange',
      'purple',
      'pink',
      'gray',
      'grey',
      'brown',
      'cyan',
      'magenta',
      'lime',
      'indigo',
      'violet',
      'gold',
      'silver',
      'navy',
      'teal',
      'olive',
      'maroon',
      'aqua',
      'fuchsia',
    ]);

    const colorProperties = new Set([
      'color',
      'backgroundColor',
      'borderColor',
      'borderTopColor',
      'borderRightColor',
      'borderBottomColor',
      'borderLeftColor',
      'outlineColor',
      'textDecorationColor',
      'fill',
      'stroke',
      'caretColor',
      'accentColor',
    ]);

    function checkForHardcodedColors(node, value) {
      const hexMatches = value.match(hexPattern);
      if (hexMatches) {
        hexMatches.forEach(match => {
          context.report({
            node,
            messageId: 'hardcodedHexColor',
            data: { value: match },
          });
        });
      }

      const rgbMatches = value.match(rgbPattern);
      if (rgbMatches) {
        rgbMatches.forEach(match => {
          context.report({
            node,
            messageId: 'hardcodedRgbColor',
            data: { value: match },
          });
        });
      }

      const hslMatches = value.match(hslPattern);
      if (hslMatches) {
        hslMatches.forEach(match => {
          context.report({
            node,
            messageId: 'hardcodedHslColor',
            data: { value: match },
          });
        });
      }
    }

    function checkStyleProperty(node, propertyName, valueNode) {
      if (!colorProperties.has(propertyName)) {
        return;
      }

      if (valueNode.type === 'Literal' && typeof valueNode.value === 'string') {
        const value = valueNode.value;

        if (hexValuePattern.test(value)) {
          context.report({
            node: valueNode,
            messageId: 'hardcodedHexColor',
            data: { value },
          });
          return;
        }

        if (rgbValuePattern.test(value)) {
          context.report({
            node: valueNode,
            messageId: 'hardcodedRgbColor',
            data: { value },
          });
          return;
        }

        if (hslValuePattern.test(value)) {
          context.report({
            node: valueNode,
            messageId: 'hardcodedHslColor',
            data: { value },
          });
          return;
        }

        if (
          namedColors.has(value.toLowerCase()) &&
          !value.startsWith('var(') &&
          value !== 'currentColor' &&
          value !== 'transparent' &&
          value !== 'inherit'
        ) {
          context.report({
            node: valueNode,
            messageId: 'hardcodedNamedColor',
            data: { value, property: propertyName },
          });
        }
      }
    }

    return {
      JSXAttribute(node) {
        if (
          node.name.name === 'className' &&
          node.value &&
          node.value.type === 'Literal' &&
          typeof node.value.value === 'string'
        ) {
          checkForHardcodedColors(node, node.value.value);
        }

        if (
          node.name.name === 'className' &&
          node.value &&
          node.value.type === 'JSXExpressionContainer' &&
          node.value.expression.type === 'TemplateLiteral'
        ) {
          node.value.expression.quasis.forEach(quasi => {
            checkForHardcodedColors(node, quasi.value.raw);
          });
        }

        if (
          node.name.name === 'style' &&
          node.value &&
          node.value.type === 'JSXExpressionContainer' &&
          node.value.expression.type === 'ObjectExpression'
        ) {
          node.value.expression.properties.forEach(prop => {
            if (prop.type === 'Property' && prop.key.type === 'Identifier') {
              checkStyleProperty(node, prop.key.name, prop.value);
            }
          });
        }
      },

      CallExpression(node) {
        const functionName = node.callee.name;
        if (['clsx', 'classnames', 'cn', 'cva'].includes(functionName)) {
          node.arguments.forEach(arg => {
            if (arg.type === 'Literal' && typeof arg.value === 'string') {
              checkForHardcodedColors(arg, arg.value);
            }
            if (arg.type === 'TemplateLiteral') {
              arg.quasis.forEach(quasi => {
                checkForHardcodedColors(arg, quasi.value.raw);
              });
            }
          });
        }
      },
    };
  },
};
